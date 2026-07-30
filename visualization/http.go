package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func newHTTPHandler(server *Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", newFrontendHandler())
	mux.HandleFunc("/api/fsms", server.handleFSMList)
	mux.HandleFunc("/api/fsms/", server.handleFSMDetail)
	mux.HandleFunc("/api/targets", server.handleClientServerTargets)
	mux.HandleFunc("/api/clients/", server.handleExportedClient)
	mux.HandleFunc("/api/events", server.handleEvents)
	mux.HandleFunc("/api/discoveries", server.handleDiscoveries)
	mux.HandleFunc("/api/discoveries/", server.handleDiscoveryDetail)
	mux.HandleFunc("/api/push/", server.handlePush)
	mux.HandleFunc("/api/replay/sources", server.handleReplaySources)
	mux.HandleFunc("/api/replay/sources/", server.handleReplaySourceDetail)
	mux.HandleFunc("/api/replay/directories", server.handleReplayDirectories)
	mux.HandleFunc("/api/replay/ssh/directories", server.handleReplaySSHDirectories)
	mux.HandleFunc("/api/replay/records", server.handleReplayRecords)
	mux.HandleFunc("/api/replay/records/", server.handleReplayRecordDetail)

	return mux
}

// Handler 返回可挂载到已有 HTTP Server 的处理器。
func (server *Server) Handler() http.Handler { return server.handler }

// Start 在配置地址上启动后台 HTTP Server。
func (server *Server) Start() error {
	server.lifecycleMu.Lock()
	defer server.lifecycleMu.Unlock()
	server.mu.RLock()
	closed := server.closed
	server.mu.RUnlock()
	if closed {
		return ErrServerClosed
	}
	if server.httpServer != nil {
		return ErrServerStarted
	}
	listener, err := net.Listen("tcp", server.address)
	if err != nil {
		return fmt.Errorf("listen visualization server: %w", err)
	}
	server.listener = listener
	server.httpServer = &http.Server{Handler: server.handler}
	go func(httpServer *http.Server, listener net.Listener) {
		_ = httpServer.Serve(listener)
	}(server.httpServer, listener)

	return nil
}

// Serve 使用调用方提供的 Listener 阻塞运行 Server。
func (server *Server) Serve(listener net.Listener) error {
	if listener == nil {
		return errors.New("visualization listener is nil")
	}
	server.lifecycleMu.Lock()
	server.mu.RLock()
	closed := server.closed
	server.mu.RUnlock()
	if closed {
		server.lifecycleMu.Unlock()

		return ErrServerClosed
	}
	if server.httpServer != nil {
		server.lifecycleMu.Unlock()

		return ErrServerStarted
	}
	server.listener = listener
	server.httpServer = &http.Server{Handler: server.handler}
	httpServer := server.httpServer
	server.lifecycleMu.Unlock()
	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve visualization server: %w", err)
	}

	return nil
}

// URL 返回 Start 或 Serve 后的访问地址；尚未启动时返回空字符串。
func (server *Server) URL() string {
	server.lifecycleMu.Lock()
	defer server.lifecycleMu.Unlock()
	if server.listener == nil {
		return ""
	}
	host, port, err := net.SplitHostPort(server.listener.Addr().String())
	if err != nil {
		return "http://" + server.listener.Addr().String()
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	return "http://" + net.JoinHostPort(host, port)
}

func (server *Server) broadcast(eventType string, id string) {
	payload, err := json.Marshal(streamEvent{Type: eventType, FSMID: id, Timestamp: time.Now()})
	if err != nil {
		return
	}
	server.mu.RLock()
	defer server.mu.RUnlock()
	for client := range server.clients {
		select {
		case client <- payload:
		default:
		}
	}
}

func (server *Server) handleFSMList(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, server.Snapshots())
	case http.MethodPost:
		server.handleRemoteRegistration(writer, request)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (server *Server) handleFSMDetail(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/fsms/")
	snapshot, exists := server.Snapshot(id)
	if !exists {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "fsm not found"})

		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

type remoteRegistrationRequest struct {
	Definition
	DisplayName string `json:"displayName"`
	Path        string `json:"path,omitempty"`
}

func (server *Server) registerRemote(definition Definition, config RegistrationConfig) error {
	config.Mode = ModePush
	normalizedDefinition, normalizedConfig, err := normalizeRegistration(definition, config, true)
	if err != nil {
		return err
	}
	_, err = server.addRegistration(
		context.Background(),
		normalizedDefinition,
		normalizedConfig,
		nil,
	)

	return err
}

// handleRemoteRegistration 注册跨进程 push 客户端的完整静态 Definition 和挂载配置。
// 后续 POST /api/push/{id} 只接收 RealtimeData。
func (server *Server) handleRemoteRegistration(writer http.ResponseWriter, request *http.Request) {
	var registration remoteRegistrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4<<20)).Decode(&registration); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}
	config := RegistrationConfig{
		DisplayName: registration.DisplayName,
		Path:        registration.Path,
		Mode:        ModePush,
	}
	if err := server.registerRemote(registration.Definition, config); err != nil {
		writeJSON(writer, http.StatusConflict, map[string]string{"error": err.Error()})

		return
	}
	stored, _ := server.Snapshot(strings.TrimSpace(registration.ID))
	writeJSON(writer, http.StatusCreated, stored)
}

// handlePush 提供类似 Pushgateway 的实时数据入口。
// FSM 必须先通过 POST /api/fsms 注册完整 Definition。
func (server *Server) handlePush(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(request.URL.Path, "/api/push/"))
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, string(rune(92))) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": ErrInvalidRegistrationID.Error()})

		return
	}
	var wire realtimeDataWire
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}
	data := wire.realtimeData()
	if err := server.Push(request.Context(), id, data); err != nil {
		status := http.StatusConflict
		if errors.Is(err, ErrRegistrationNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(writer, status, map[string]string{"error": err.Error()})

		return
	}
	stored, _ := server.Snapshot(id)
	writeJSON(writer, http.StatusAccepted, stored)
}

func (server *Server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)

		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")

	client := make(chan []byte, 16)
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		http.Error(writer, "server closed", http.StatusServiceUnavailable)

		return
	}
	server.clients[client] = struct{}{}
	server.mu.Unlock()
	defer func() {
		server.mu.Lock()
		if _, exists := server.clients[client]; exists {
			delete(server.clients, client)
			close(client)
		}
		server.mu.Unlock()
	}()

	initial, _ := json.Marshal(streamEvent{Type: "connected", Timestamp: time.Now()})
	if !writeSSE(writer, flusher, initial) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case payload, open := <-client:
			if !open || !writeSSE(writer, flusher, payload) {
				return
			}
		case <-heartbeat.C:
			if _, err := writer.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-request.Context().Done():
			return
		}
	}
}

func writeSSE(writer http.ResponseWriter, flusher http.Flusher, payload []byte) bool {
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
		return false
	}
	flusher.Flush()

	return true
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
