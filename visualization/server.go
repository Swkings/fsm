package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// ServerOption 配置可视化 Server。
type ServerOption func(*Server)

// WithAddress 设置监听地址。默认值为 127.0.0.1:0。
func WithAddress(address string) ServerOption {
	return func(server *Server) {
		if strings.TrimSpace(address) != "" {
			server.address = address
		}
	}
}

// WithTraceLimit 设置每个实例保留的最近 trace 数量。默认保留 100 条。
func WithTraceLimit(limit int) ServerOption {
	return func(server *Server) {
		if limit > 0 {
			server.traceLimit = limit
		}
	}
}

type registeredSource struct {
	config   registrationConfig
	mode     Mode
	pull     PullSource
	snapshot Snapshot
	traces   []TraceSnapshot
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

type streamEvent struct {
	Type      string    `json:"type"`
	FSMID     string    `json:"fsmId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Server 提供第三方 FSM 注册、pull/push 采集、HTTP API、SSE 和内嵌 Web 页面。
// 网络服务只有在调用 Start、Serve 或将 Handler 挂载到已有 HTTP Server 时才启用。
type Server struct {
	address    string
	traceLimit int
	handler    http.Handler

	mu      sync.RWMutex
	sources map[string]*registeredSource
	clients map[chan []byte]struct{}
	closed  bool

	lifecycleMu sync.Mutex
	httpServer  *http.Server
	listener    net.Listener
}

// NewServer 创建未启动的独立可视化 Server。
func NewServer(options ...ServerOption) *Server {
	server := &Server{
		address:    "127.0.0.1:0",
		traceLimit: 100,
		sources:    make(map[string]*registeredSource),
		clients:    make(map[chan []byte]struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(server)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleIndex)
	mux.HandleFunc("/api/fsms", server.handleFSMList)
	mux.HandleFunc("/api/fsms/", server.handleFSMDetail)
	mux.HandleFunc("/api/events", server.handleEvents)
	mux.HandleFunc("/api/push/", server.handlePush)
	server.handler = mux
	return server
}

// Register 调用第三方对象自己的 RegisterVisualization 方法。
// 实现方可在回调中自由选择 pull 或 push 模式。
func (server *Server) Register(ctx context.Context, target Registrable) (Registration, error) {
	if isNilInterface(target) {
		return nil, ErrInvalidSource
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return target.RegisterVisualization(ctx, server)
}

// RegisterPull 注册由 Server 周期性主动采集的数据源。
// 注册时会立即采集一次，之后按 WithPullInterval 配置串行采集。
func (server *Server) RegisterPull(
	ctx context.Context,
	source PullSource,
	options ...RegistrationOption,
) (Registration, error) {
	if isNilInterface(source) {
		return nil, ErrInvalidSource
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := applyRegistrationOptions(source.Descriptor(), options)
	if err != nil {
		return nil, err
	}
	registered, err := server.addRegistration(ctx, config, ModePull, source)
	if err != nil {
		return nil, err
	}
	handle := &registrationHandle{
		server: server,
		id:     config.descriptor.ID,
		mode:   ModePull,
		done:   registered.done,
	}
	server.collectPull(registered.ctx, config.descriptor.ID)
	go server.runPull(config.descriptor.ID, registered)
	return handle, nil
}

// RegisterPush 创建客户端主动推送数据的注册，并返回 Pusher。
func (server *Server) RegisterPush(
	ctx context.Context,
	descriptor Descriptor,
	options ...RegistrationOption,
) (Pusher, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := applyRegistrationOptions(descriptor, options)
	if err != nil {
		return nil, err
	}
	registered, err := server.addRegistration(ctx, config, ModePush, nil)
	if err != nil {
		return nil, err
	}
	handle := &registrationHandle{
		server: server,
		id:     config.descriptor.ID,
		mode:   ModePush,
		done:   registered.done,
	}
	return &pushHandle{registrationHandle: handle}, nil
}

func (server *Server) addRegistration(
	ctx context.Context,
	config registrationConfig,
	mode Mode,
	pull PullSource,
) (*registeredSource, error) {
	sourceContext, cancel := context.WithCancel(ctx)
	snapshot := Snapshot{
		Descriptor: config.descriptor,
		Mode:       mode,
		Payload:    json.RawMessage("null"),
		UpdatedAt:  time.Now(),
	}
	registered := &registeredSource{
		config:   config,
		mode:     mode,
		pull:     pull,
		snapshot: snapshot,
		ctx:      sourceContext,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		cancel()
		return nil, ErrServerClosed
	}
	if _, exists := server.sources[config.descriptor.ID]; exists {
		server.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("%w: %s", ErrRegistrationIDRepeated, config.descriptor.ID)
	}
	server.sources[config.descriptor.ID] = registered
	server.mu.Unlock()
	server.broadcast("registered", config.descriptor.ID)
	go func() {
		<-sourceContext.Done()
		server.Unregister(config.descriptor.ID)
	}()
	return registered, nil
}

func (server *Server) runPull(id string, registered *registeredSource) {
	ticker := time.NewTicker(registered.config.pullInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			server.collectPull(registered.ctx, id)
		case <-registered.done:
			return
		}
	}
}

func (server *Server) collectPull(ctx context.Context, id string) {
	server.mu.RLock()
	registered, exists := server.sources[id]
	if !exists || registered.mode != ModePull || registered.pull == nil {
		server.mu.RUnlock()
		return
	}
	source := registered.pull
	config := registered.config
	server.mu.RUnlock()

	snapshot, err := collectSnapshot(source, ctx)
	if err != nil {
		server.mu.Lock()
		current, stillRegistered := server.sources[id]
		if stillRegistered && current == registered {
			current.snapshot.CollectionError = err.Error()
			current.snapshot.UpdatedAt = time.Now()
		}
		server.mu.Unlock()
		if stillRegistered {
			server.broadcast("updated", id)
		}
		return
	}
	snapshot.CollectionError = ""
	server.storeSnapshot(id, registered, prepareSnapshot(config, ModePull, snapshot), nil)
}

func collectSnapshot(source PullSource, ctx context.Context) (snapshot Snapshot, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("pull source panic: %v", recovered)
		}
	}()
	return source.Collect(ctx)
}

// Push 向已存在的 push 模式注册提交快照和可选的增量 trace。
func (server *Server) Push(
	ctx context.Context,
	id string,
	snapshot Snapshot,
	traces ...TraceSnapshot,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	server.mu.RLock()
	registered, exists := server.sources[id]
	if !exists {
		server.mu.RUnlock()
		return fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	if registered.mode != ModePush {
		server.mu.RUnlock()
		return fmt.Errorf("%w: %s is %s", ErrRegistrationMode, id, registered.mode)
	}
	config := registered.config
	server.mu.RUnlock()
	server.storeSnapshot(id, registered, prepareSnapshot(config, ModePush, snapshot), traces)
	return nil
}

func (server *Server) storeSnapshot(
	id string,
	expected *registeredSource,
	snapshot Snapshot,
	incrementalTraces []TraceSnapshot,
) {
	server.mu.Lock()
	registered, exists := server.sources[id]
	if !exists || registered != expected {
		server.mu.Unlock()
		return
	}
	if snapshot.Traces != nil {
		registered.traces = append([]TraceSnapshot(nil), snapshot.Traces...)
	}
	if len(incrementalTraces) > 0 {
		registered.traces = append(registered.traces, incrementalTraces...)
	}
	if len(registered.traces) > server.traceLimit {
		registered.traces = append(
			[]TraceSnapshot(nil),
			registered.traces[len(registered.traces)-server.traceLimit:]...,
		)
	}
	snapshot.Traces = append([]TraceSnapshot(nil), registered.traces...)
	registered.snapshot = snapshot
	server.mu.Unlock()
	server.broadcast("updated", id)
}

func prepareSnapshot(config registrationConfig, mode Mode, snapshot Snapshot) Snapshot {
	snapshot = cloneSnapshot(snapshot)
	snapshot.Descriptor = config.descriptor
	snapshot.Mode = mode
	if config.payloadProvider != nil {
		snapshot.Payload, snapshot.PayloadError = snapshotPayload(config.payloadProvider)
	} else if snapshot.Payload == nil {
		snapshot.Payload = json.RawMessage("null")
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now()
	}
	completeTopology(&snapshot)
	return snapshot
}

func completeTopology(snapshot *Snapshot) {
	stateSet := make(map[string]struct{}, len(snapshot.States)+1)
	for _, state := range snapshot.States {
		stateSet[state] = struct{}{}
	}
	stateSet[snapshot.CurrentState] = struct{}{}
	if snapshot.AvailableTransitions == nil {
		snapshot.AvailableTransitions = make([]TransitionSnapshot, 0)
	}
	for _, transition := range snapshot.Transitions {
		stateSet[transition.FromState] = struct{}{}
		stateSet[transition.ToState] = struct{}{}
		if transition.FromState == snapshot.CurrentState && !containsTransition(snapshot.AvailableTransitions, transition) {
			snapshot.AvailableTransitions = append(snapshot.AvailableTransitions, transition)
		}
	}
	snapshot.States = snapshot.States[:0]
	for state := range stateSet {
		snapshot.States = append(snapshot.States, state)
	}
	sort.Strings(snapshot.States)
}

func containsTransition(transitions []TransitionSnapshot, candidate TransitionSnapshot) bool {
	for _, transition := range transitions {
		if transition == candidate {
			return true
		}
	}
	return false
}

type registrationHandle struct {
	server *Server
	id     string
	mode   Mode
	done   <-chan struct{}
}

func (handle *registrationHandle) ID() string            { return handle.id }
func (handle *registrationHandle) Mode() Mode            { return handle.mode }
func (handle *registrationHandle) Done() <-chan struct{} { return handle.done }
func (handle *registrationHandle) Unregister() bool {
	return handle.server != nil && handle.server.Unregister(handle.id)
}

type pushHandle struct {
	*registrationHandle
}

func (handle *pushHandle) Push(
	ctx context.Context,
	snapshot Snapshot,
	traces ...TraceSnapshot,
) error {
	if handle == nil || handle.server == nil {
		return ErrRegistrationNotFound
	}
	return handle.server.Push(ctx, handle.id, snapshot, traces...)
}

// Unregister 动态移除指定注册，并结束 pull worker 或 push 句柄。
func (server *Server) Unregister(id string) bool {
	server.mu.Lock()
	registered, exists := server.sources[id]
	if exists {
		delete(server.sources, id)
	}
	server.mu.Unlock()
	if !exists {
		return false
	}
	stopRegistration(registered)
	server.broadcast("unregistered", id)
	return true
}

func stopRegistration(registered *registeredSource) {
	registered.cancel()
	select {
	case <-registered.done:
	default:
		close(registered.done)
	}
}

// Snapshots 返回全部深拷贝快照，并按 path、显示名称和 ID 稳定排序。
func (server *Server) Snapshots() []Snapshot {
	server.mu.RLock()
	snapshots := make([]Snapshot, 0, len(server.sources))
	for _, registered := range server.sources {
		snapshots = append(snapshots, cloneSnapshot(registered.snapshot))
	}
	server.mu.RUnlock()
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Path != snapshots[j].Path {
			return snapshots[i].Path < snapshots[j].Path
		}
		if snapshots[i].DisplayName != snapshots[j].DisplayName {
			return snapshots[i].DisplayName < snapshots[j].DisplayName
		}
		return snapshots[i].ID < snapshots[j].ID
	})
	return snapshots
}

// Snapshot 返回指定注册 ID 的深拷贝快照。
func (server *Server) Snapshot(id string) (Snapshot, bool) {
	server.mu.RLock()
	registered, exists := server.sources[id]
	if !exists {
		server.mu.RUnlock()
		return Snapshot{}, false
	}
	snapshot := cloneSnapshot(registered.snapshot)
	server.mu.RUnlock()
	return snapshot, true
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

// Close 停止 HTTP 服务、SSE、pull worker，并使全部注册句柄 Done。
func (server *Server) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		return nil
	}
	server.closed = true
	registered := make([]*registeredSource, 0, len(server.sources))
	for _, source := range server.sources {
		registered = append(registered, source)
	}
	server.sources = make(map[string]*registeredSource)
	clients := make([]chan []byte, 0, len(server.clients))
	for client := range server.clients {
		clients = append(clients, client)
		delete(server.clients, client)
	}
	server.mu.Unlock()
	for _, source := range registered {
		stopRegistration(source)
	}
	for _, client := range clients {
		close(client)
	}

	server.lifecycleMu.Lock()
	httpServer := server.httpServer
	server.lifecycleMu.Unlock()
	if httpServer == nil {
		return nil
	}
	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown visualization server: %w", err)
	}
	return nil
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

// handlePush 提供类似 Pushgateway 的 HTTP push 入口。
// 首次 POST /api/push/{id} 会使用请求中的 Descriptor 自动创建 push 注册。
func (server *Server) handlePush(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(request.URL.Path, "/api/push/"))
	if id == "" || strings.ContainsAny(id, "/\\") {
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
		if _, err := server.RegisterPush(context.Background(), snapshot.Descriptor); err != nil &&
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

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
