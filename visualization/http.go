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
	mux.HandleFunc("/", server.handleIndex)
	mux.HandleFunc("/api/fsms", server.handleFSMList)
	mux.HandleFunc("/api/fsms/", server.handleFSMDetail)
	mux.HandleFunc("/api/events", server.handleEvents)
	mux.HandleFunc("/api/push/", server.handlePush)
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

func (server *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write([]byte(webPage))
}

func (server *Server) handleFSMList(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, server.Snapshots())
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

func (server *Server) registerRemote(descriptor Descriptor) error {
	config, err := normalizeClientConfig(ClientConfig{
		ID:          descriptor.ID,
		Name:        descriptor.Name,
		DisplayName: descriptor.DisplayName,
		Description: descriptor.Description,
		Path:        descriptor.Path,
		Mode:        ModePush,
	}, true)
	if err != nil {
		return err
	}
	_, err = server.addRegistration(context.Background(), config, nil)
	return err
}

// handlePush 提供类似 Pushgateway 的 HTTP push 入口。
// 首次 POST /api/push/{id} 会使用请求中的 Descriptor 自动创建注册。
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
	var snapshot Snapshot
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4<<20)).Decode(&snapshot); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, exists := server.Snapshot(id); !exists {
		snapshot.ID = id
		if err := server.registerRemote(snapshot.Descriptor); err != nil &&
			!errors.Is(err, ErrRegistrationIDRepeated) {
			writeJSON(writer, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := server.Push(request.Context(), id, snapshot); err != nil {
		writeJSON(writer, http.StatusConflict, map[string]string{"error": err.Error()})
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
