package visualization

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type registeredClient struct {
	definition  Definition
	config      RegistrationConfig
	client      Client
	snapshot    Snapshot
	traces      []TraceSnapshot
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	unsubscribe func()
	updateMu    sync.Mutex
}

type streamEvent struct {
	Type      string    `json:"type"`
	FSMID     string    `json:"fsmId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Registration 是 Register 返回的活动注册句柄。
type Registration struct {
	server *Server
	id     string
	mode   Mode
	done   <-chan struct{}
}

// ID 返回稳定注册 ID。
func (registration *Registration) ID() string { return registration.id }

// Mode 返回 pull 或 push 工作模式。
func (registration *Registration) Mode() Mode { return registration.mode }

// Done 在 context 取消、主动注销或 Server 关闭时关闭。
func (registration *Registration) Done() <-chan struct{} { return registration.done }

// Unregister 动态移除注册。
func (registration *Registration) Unregister() bool {
	return registration != nil && registration.server != nil && registration.server.Unregister(registration.id)
}

// Server 提供客户端注册、pull/push 实时数据采集、HTTP API、SSE 和内嵌 Web 页面。
type Server struct {
	address    string
	traceLimit int
	handler    http.Handler

	mu      sync.RWMutex
	sources map[string]*registeredClient
	clients map[chan []byte]struct{}
	closed  bool

	discoveryMu sync.RWMutex
	discoveries map[string]*registeredDiscovery

	lifecycleMu sync.Mutex
	httpServer  *http.Server
	listener    net.Listener
}

// NewServer 使用默认配置创建未启动的 Server。
func NewServer() *Server { return NewServerWithConfig(ServerConfig{}) }

// NewServerWithConfig 使用一个普通配置结构创建未启动的 Server。
func NewServerWithConfig(config ServerConfig) *Server {
	if address := strings.TrimSpace(config.Address); address != "" {
		config.Address = address
	} else {
		config.Address = "0.0.0.0:5005"
	}
	if config.TraceLimit <= 0 {
		config.TraceLimit = 100
	}
	server := &Server{
		address:     config.Address,
		traceLimit:  config.TraceLimit,
		sources:     make(map[string]*registeredClient),
		clients:     make(map[chan []byte]struct{}),
		discoveries: make(map[string]*registeredDiscovery),
	}
	server.handler = newHTTPHandler(server)
	return server
}

// Register 注册客户端。
// Server 在此阶段只读取一次完整 Definition，并将 config 绑定到返回的 Registration。
// 后续 pull/push 只接收 RealtimeData，不会把注册配置写回客户端。
func (server *Server) Register(ctx context.Context, client Client, config RegistrationConfig) (*Registration, error) {
	if isNilInterface(client) {
		return nil, ErrInvalidClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pushClient, supportsPush := client.(PushClient)
	definition, err := clientDefinition(client)
	if err != nil {
		return nil, err
	}
	definition, config, err = normalizeRegistration(definition, config, supportsPush)
	if err != nil {
		return nil, err
	}
	registered, err := server.addRegistration(ctx, definition, config, client)
	if err != nil {
		return nil, err
	}
	handle := &Registration{
		server: server,
		id:     definition.ID,
		mode:   config.Mode,
		done:   registered.done,
	}
	if config.Mode == ModePush {
		unsubscribe, subscribeErr := subscribeClient(pushClient, func(data RealtimeData, traces ...TraceSnapshot) error {
			return server.storeRealtimeData(definition.ID, registered, data, traces)
		})
		if subscribeErr != nil {
			handle.Unregister()
			return nil, subscribeErr
		}
		server.attachUnsubscribe(definition.ID, registered, unsubscribe)
	}
	_ = server.refreshClient(definition.ID, registered)
	if config.Mode == ModePull {
		go server.runPull(definition.ID, registered)
	}
	return handle, nil
}

func clientDefinition(client Client) (definition Definition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: definition panic: %v", ErrInvalidClient, recovered)
		}
	}()
	return client.VisualizationDefinition(), nil
}

func subscribeClient(client PushClient, publish PublishFunc) (unsubscribe func(), err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: subscribe panic: %v", ErrInvalidClient, recovered)
		}
	}()
	return client.SubscribeVisualization(publish), nil
}

func (server *Server) addRegistration(
	ctx context.Context,
	definition Definition,
	config RegistrationConfig,
	client Client,
) (*registeredClient, error) {
	sourceContext, cancel := context.WithCancel(ctx)
	registered := &registeredClient{
		definition: cloneDefinition(definition),
		config:     config,
		client:     client,
		snapshot: buildSnapshot(definition, config, RealtimeData{
			Payload:   json.RawMessage("null"),
			UpdatedAt: time.Now(),
		}, nil, ""),
		ctx:    sourceContext,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		cancel()
		return nil, ErrServerClosed
	}
	if _, exists := server.sources[definition.ID]; exists {
		server.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("%w: %s", ErrRegistrationIDRepeated, definition.ID)
	}
	server.sources[definition.ID] = registered
	server.mu.Unlock()
	server.broadcast("registered", definition.ID)
	go func() {
		<-sourceContext.Done()
		server.Unregister(definition.ID)
	}()
	return registered, nil
}

func (server *Server) attachUnsubscribe(id string, expected *registeredClient, unsubscribe func()) {
	if unsubscribe == nil {
		return
	}
	server.mu.Lock()
	current, exists := server.sources[id]
	if exists && current == expected {
		current.unsubscribe = unsubscribe
	}
	server.mu.Unlock()
	if !exists || current != expected {
		safeUnsubscribe(unsubscribe)
	}
}

func (server *Server) runPull(id string, registered *registeredClient) {
	ticker := time.NewTicker(registered.config.PullInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = server.refreshClient(id, registered)
		case <-registered.done:
			return
		}
	}
}

func (server *Server) refreshClient(id string, expected *registeredClient) error {
	expected.updateMu.Lock()
	defer expected.updateMu.Unlock()
	if err := expected.ctx.Err(); err != nil {
		return err
	}
	data, err := clientRealtimeData(expected.client, expected.ctx)
	if err != nil {
		server.markCollectionError(id, expected, err)
		return err
	}
	if !server.storeRealtimeDataLocked(id, expected, data, nil) {
		return fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	return nil
}

func clientRealtimeData(client Client, ctx context.Context) (data RealtimeData, err error) {
	if client == nil {
		return RealtimeData{}, ErrInvalidClient
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("visualization realtime data panic: %v", recovered)
		}
	}()
	return client.VisualizationSnapshot(ctx)
}

func (server *Server) markCollectionError(id string, expected *registeredClient, err error) {
	server.mu.Lock()
	current, exists := server.sources[id]
	if exists && current == expected {
		current.snapshot.CollectionError = err.Error()
		current.snapshot.UpdatedAt = time.Now()
	}
	server.mu.Unlock()
	if exists && current == expected {
		server.broadcast("updated", id)
	}
}

// Push 为 HTTP Pushgateway 或其他跨进程客户端提交实时数据。
// FSM 的 Definition 必须已经完成注册，实时更新中不再接受静态定义。
func (server *Server) Push(
	ctx context.Context,
	id string,
	data RealtimeData,
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
	if registered.config.Mode != ModePush {
		server.mu.RUnlock()
		return fmt.Errorf("%w: %s is %s", ErrRegistrationMode, id, registered.config.Mode)
	}
	server.mu.RUnlock()
	return server.storeRealtimeData(id, registered, data, traces)
}

func (server *Server) storeRealtimeData(
	id string,
	expected *registeredClient,
	data RealtimeData,
	incrementalTraces []TraceSnapshot,
) error {
	expected.updateMu.Lock()
	defer expected.updateMu.Unlock()
	if err := expected.ctx.Err(); err != nil {
		return err
	}
	if !server.storeRealtimeDataLocked(id, expected, data, incrementalTraces) {
		return fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	return nil
}

func (server *Server) storeRealtimeDataLocked(
	id string,
	expected *registeredClient,
	data RealtimeData,
	incrementalTraces []TraceSnapshot,
) bool {
	data = cloneRealtimeData(data)
	server.mu.Lock()
	registered, exists := server.sources[id]
	if !exists || registered != expected {
		server.mu.Unlock()
		return false
	}
	if data.Traces != nil {
		registered.traces = append([]TraceSnapshot(nil), data.Traces...)
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
	registered.snapshot = buildSnapshot(registered.definition, registered.config, data, registered.traces, "")
	server.mu.Unlock()
	server.broadcast("updated", id)
	return true
}

func buildSnapshot(
	definition Definition,
	config RegistrationConfig,
	data RealtimeData,
	traces []TraceSnapshot,
	collectionError string,
) Snapshot {
	data = cloneRealtimeData(data)
	if data.Payload == nil {
		data.Payload = json.RawMessage("null")
	}
	if data.UpdatedAt.IsZero() {
		data.UpdatedAt = time.Now()
	}
	available := make([]TransitionSnapshot, 0)
	for _, transition := range definition.Transitions {
		if transition.FromState == data.CurrentState {
			available = append(available, transition)
		}
	}
	return Snapshot{
		Descriptor:           definition.Descriptor(config),
		Mode:                 config.Mode,
		TransitionID:         data.TransitionID,
		CurrentState:         data.CurrentState,
		PreviousState:        data.PreviousState,
		PreviousEvent:        data.PreviousEvent,
		ProcessingEvent:      data.ProcessingEvent,
		Payload:              data.Payload,
		PayloadError:         data.PayloadError,
		CollectionError:      collectionError,
		States:               append([]string(nil), definition.States...),
		Events:               append([]string(nil), definition.Events...),
		Transitions:          append([]TransitionSnapshot(nil), definition.Transitions...),
		AvailableTransitions: available,
		Traces:               append([]TraceSnapshot(nil), traces...),
		UpdatedAt:            data.UpdatedAt,
	}
}

// Unregister 动态移除指定注册。
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

func stopRegistration(registered *registeredClient) {
	registered.cancel()
	safeUnsubscribe(registered.unsubscribe)
	select {
	case <-registered.done:
	default:
		close(registered.done)
	}
}

func safeUnsubscribe(unsubscribe func()) {
	if unsubscribe == nil {
		return
	}
	defer func() { _ = recover() }()
	unsubscribe()
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
	registered := make([]*registeredClient, 0, len(server.sources))
	for _, source := range server.sources {
		registered = append(registered, source)
	}
	server.sources = make(map[string]*registeredClient)
	clients := make([]chan []byte, 0, len(server.clients))
	for client := range server.clients {
		clients = append(clients, client)
		delete(server.clients, client)
	}
	server.mu.Unlock()
	for _, source := range registered {
		stopRegistration(source)
	}
	server.stopAllDiscoveries()
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
