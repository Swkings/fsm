// Package server provides the configuration-driven FSM service lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swkings/fsm/config"
	fsmmcp "github.com/swkings/fsm/mcp"
	"github.com/swkings/fsm/store"
	"github.com/swkings/fsm/visualization"
	"github.com/zeromicro/go-zero/core/service"
)

var (
	ErrInvalidManagedFSM    = errors.New("invalid managed fsm")
	ErrManagedFSMRegistered = errors.New("fsm is already registered to server")
	ErrServerStopped        = errors.New("fsm server is stopped")
	ErrStoreNotConfigured   = errors.New("fsm store is not configured")
)

var _ service.Service = (*Server)(nil)

// RegisterableFSMI 是可由 Server 统一管理的 FSM 能力集合。
// *FSM[T] 已实现该接口，不要求业务方额外编写适配器。
type RegisterableFSMI interface {
	visualization.PushClientI
	VisualizationRegistrationConfig() visualization.RegistrationConfig
	BindStore(context.Context, store.StoreI, string) error
	UnbindStore()
}

type managedFSMRegistration struct {
	machine       RegisterableFSMI
	visualization *visualization.Registration
	storeBound    bool
}

// Server 统一管理 FSM 持久化、可视化、MCP 查询和注册生命周期。
// Start 与 Stop 实现 github.com/zeromicro/go-zero/core/service.Service。
type Server struct {
	config               config.Config
	store                store.StoreI
	visualization        *visualization.Server
	visualizationEnabled bool
	mcpEnabled           bool
	mcp                  *mcpsdk.Server
	mcpHandler           http.Handler

	ctx    context.Context
	cancel context.CancelFunc

	registrationMu sync.Mutex
	registrations  map[string]*managedFSMRegistration

	lifecycleMu   sync.Mutex
	startOnce     sync.Once
	stopOnce      sync.Once
	started       chan struct{}
	done          chan struct{}
	startErr      error
	stopErr       error
	mcpHTTPServer *http.Server
	mcpListener   net.Listener
}

// NewServer 根据 FSM 顶层配置创建统一服务，但不监听端口。
func NewServer(cfg config.Config) (*Server, error) {
	visualizationEnabled := strings.TrimSpace(cfg.Visualization.ServerAddress) != ""
	mcpEnabled := cfg.MCP.Enabled || strings.TrimSpace(cfg.MCP.ServerAddress) != ""
	if mcpEnabled && fsmmcp.Endpoint(cfg.MCP.Endpoint) == "/healthz" {
		return nil, errors.New("fsm mcp endpoint conflicts with /healthz")
	}
	var backend store.StoreI
	var err error
	if storeConfigured(cfg.Store) {
		backend, err = store.New(cfg.Store)
		if err != nil {
			return nil, fmt.Errorf("create fsm store: %w", err)
		}
	}
	var registry *visualization.Server
	if visualizationEnabled || mcpEnabled {
		registry = visualization.NewServerWithConfig(config.VisualizationServer{
			Address:    cfg.Visualization.ServerAddress,
			TraceLimit: cfg.Visualization.TraceLimit,
		})
		if backend != nil && visualizationEnabled {
			if err = registry.RegisterReplayStore("primary", "FSM Store", backend); err != nil {
				_ = registry.Close(context.Background())
				_ = backend.Close()
				return nil, fmt.Errorf("register fsm replay store: %w", err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		config:               cfg,
		store:                backend,
		visualization:        registry,
		visualizationEnabled: visualizationEnabled,
		mcpEnabled:           mcpEnabled,
		ctx:                  ctx,
		cancel:               cancel,
		registrations:        make(map[string]*managedFSMRegistration),
		started:              make(chan struct{}),
		done:                 make(chan struct{}),
	}
	if server.mcpEnabled {
		server.mcp, server.mcpHandler = fsmmcp.NewServer(registry, cfg.MCP)
	}
	return server, nil
}

// MustNewServer 与 NewServer 相同，但已配置的子服务初始化失败时 panic。
func MustNewServer(cfg config.Config) *Server {
	server, err := NewServer(cfg)
	if err != nil {
		panic(err)
	}
	return server
}

// Register 注册一个或多个 FSM，并按配置接入 Store、可视化和 MCP 查询能力。
func (server *Server) Register(machines ...RegisterableFSMI) error {
	registered := make([]string, 0, len(machines))
	for _, machine := range machines {
		id, err := server.register(machine, "")
		if err != nil {
			for i := len(registered) - 1; i >= 0; i-- {
				server.Unregister(registered[i])
			}
			return err
		}
		registered = append(registered, id)
	}
	return nil
}

// RegisterWithKey 使用调用方提供的稳定业务 key 绑定 Store，适合重启恢复。
func (server *Server) RegisterWithKey(storeKey string, machine RegisterableFSMI) error {
	if server.store == nil {
		return ErrStoreNotConfigured
	}
	_, err := server.register(machine, storeKey)
	return err
}

func (server *Server) register(machine RegisterableFSMI, storeKey string) (string, error) {
	server.lifecycleMu.Lock()
	defer server.lifecycleMu.Unlock()
	if isNilManagedFSM(machine) {
		return "", ErrInvalidManagedFSM
	}
	select {
	case <-server.done:
		return "", ErrServerStopped
	default:
	}
	definition := machine.VisualizationDefinition()
	server.registrationMu.Lock()
	if _, exists := server.registrations[definition.ID]; exists {
		server.registrationMu.Unlock()
		return "", fmt.Errorf("%w: %s", ErrManagedFSMRegistered, definition.ID)
	}
	server.registrationMu.Unlock()
	registrationConfig := machine.VisualizationRegistrationConfig()
	storeBound := false
	if server.store != nil {
		if strings.TrimSpace(storeKey) == "" {
			storeKey = defaultStoreKey(definition, registrationConfig)
		}
		if err := machine.BindStore(server.ctx, server.store, storeKey); err != nil {
			return "", fmt.Errorf("bind fsm %q store: %w", definition.Name, err)
		}
		storeBound = true
		definition = machine.VisualizationDefinition()
		registrationConfig = machine.VisualizationRegistrationConfig()
	}
	id := definition.ID
	var registration *visualization.Registration
	if server.visualization != nil {
		registrationConfig.Path = joinRegistrationPath(server.config.Visualization.PathPrefix, registrationConfig.Path)
		var err error
		registration, err = server.visualization.Register(server.ctx, machine, registrationConfig)
		if err != nil {
			if storeBound {
				machine.UnbindStore()
			}
			return "", fmt.Errorf("register fsm %q: %w", definition.Name, err)
		}
		id = registration.ID()
	}
	server.registrationMu.Lock()
	if _, exists := server.registrations[id]; exists {
		server.registrationMu.Unlock()
		if registration != nil {
			registration.Unregister()
		}
		if storeBound {
			machine.UnbindStore()
		}
		return "", fmt.Errorf("%w: %s", ErrManagedFSMRegistered, id)
	}
	server.registrations[id] = &managedFSMRegistration{machine: machine, visualization: registration, storeBound: storeBound}
	server.registrationMu.Unlock()
	return id, nil
}

// Unregister 从统一 Server 移除 FSM，并解除 Store 绑定，但不关闭 FSM 本身。
func (server *Server) Unregister(id string) bool {
	server.registrationMu.Lock()
	registration, exists := server.registrations[id]
	if exists {
		delete(server.registrations, id)
	}
	server.registrationMu.Unlock()
	if !exists {
		return false
	}
	if registration.visualization != nil {
		registration.visualization.Unregister()
	}
	if registration.storeBound {
		registration.machine.UnbindStore()
	}
	return true
}

// Start 启动配置中启用的可视化和 MCP 服务并阻塞到 Stop。
func (server *Server) Start() {
	server.startOnce.Do(func() {
		startErr := server.start()
		server.lifecycleMu.Lock()
		server.startErr = errors.Join(server.startErr, startErr)
		server.lifecycleMu.Unlock()
		close(server.started)
		if startErr != nil {
			server.stop()
		}
	})
	if err := server.Err(); err != nil {
		panic(err)
	}
	<-server.done
}

func (server *Server) start() error {
	select {
	case <-server.done:
		return ErrServerStopped
	default:
	}
	if server.visualizationEnabled {
		if err := server.visualization.Start(); err != nil {
			return fmt.Errorf("start visualization server: %w", err)
		}
	}
	if server.mcpEnabled {
		if err := server.startMCPHTTP(); err != nil {
			if server.visualizationEnabled {
				_ = server.visualization.Close(context.Background())
			}
			return err
		}
	}
	return nil
}

// Stop 停止全部子服务并释放 Store，满足 go-zero service.Service。
func (server *Server) Stop() {
	server.stopOnce.Do(server.stop)
}

func (server *Server) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// Shutdown 使用调用方 context 停止 Server；通常由 Stop 自动调用。
func (server *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	server.lifecycleMu.Lock()
	select {
	case <-server.done:
		server.lifecycleMu.Unlock()
		return server.stopErr
	default:
	}
	server.cancel()
	server.registrationMu.Lock()
	registrations := make([]*managedFSMRegistration, 0, len(server.registrations))
	for id, registration := range server.registrations {
		registrations = append(registrations, registration)
		delete(server.registrations, id)
	}
	server.registrationMu.Unlock()
	for _, registration := range registrations {
		if registration.visualization != nil {
			registration.visualization.Unregister()
		}
		if registration.storeBound {
			registration.machine.UnbindStore()
		}
	}
	var shutdownErrors []error
	if server.mcpHTTPServer != nil {
		if err := server.mcpHTTPServer.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown mcp server: %w", err))
		}
	}
	if server.visualization != nil {
		if err := server.visualization.Close(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
	}
	if server.store != nil {
		if err := server.store.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("close fsm store: %w", err))
		}
	}
	server.stopErr = errors.Join(shutdownErrors...)
	close(server.done)
	server.lifecycleMu.Unlock()
	return server.stopErr
}

func (server *Server) startMCPHTTP() error {
	address := strings.TrimSpace(server.config.MCP.ServerAddress)
	if address == "" {
		address = "127.0.0.1:5006"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen mcp server: %w", err)
	}
	server.lifecycleMu.Lock()
	server.mcpListener = listener
	server.mcpHTTPServer = &http.Server{Handler: server.mcpHandler, ReadHeaderTimeout: 10 * time.Second}
	httpServer := server.mcpHTTPServer
	server.lifecycleMu.Unlock()
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			server.lifecycleMu.Lock()
			server.startErr = errors.Join(server.startErr, fmt.Errorf("serve mcp server: %w", serveErr))
			server.lifecycleMu.Unlock()
			server.Stop()
		}
	}()
	return nil
}

// Started 在所有已配置监听服务成功启动后关闭。
func (server *Server) Started() <-chan struct{} { return server.started }

// Done 在 Server 完全停止后关闭。
func (server *Server) Done() <-chan struct{} { return server.done }

// Err 返回启动或停止阶段记录的错误。
func (server *Server) Err() error {
	server.lifecycleMu.Lock()
	defer server.lifecycleMu.Unlock()
	return errors.Join(server.startErr, server.stopErr)
}

// Store 返回 Server 持有的共享持久化后端；未配置 Store 时返回 nil。
func (server *Server) Store() store.StoreI { return server.store }

// VisualizationServer 返回内部 FSM 查询与可视化 Server；Visualization 与 MCP
// 均未配置时返回 nil。
func (server *Server) VisualizationServer() *visualization.Server { return server.visualization }

// MCPServer 返回配置启用的 MCP Server；未启用时返回 nil。
func (server *Server) MCPServer() *mcpsdk.Server { return server.mcp }

// VisualizationURL 返回可视化 HTTP 地址。
func (server *Server) VisualizationURL() string {
	if server.visualization == nil || !server.visualizationEnabled {
		return ""
	}
	return server.visualization.URL()
}

// MCPURL 返回 MCP Streamable HTTP endpoint。
func (server *Server) MCPURL() string {
	server.lifecycleMu.Lock()
	defer server.lifecycleMu.Unlock()
	if server.mcpListener == nil {
		return ""
	}
	return listenerURL(server.mcpListener, fsmmcp.Endpoint(server.config.MCP.Endpoint))
}

func defaultStoreKey(definition visualization.Definition, registration visualization.RegistrationConfig) string {
	parts := []string{strings.Trim(registration.Path, "/"), strings.Trim(definition.Name, "/"), strings.Trim(registration.DisplayName, "/")}
	key := ""
	for _, part := range parts {
		if part != "" {
			key = path.Join(key, part)
		}
	}
	if key == "" {
		key = strings.TrimSpace(definition.ID)
	}
	return key
}

func storeConfigured(cfg config.Store) bool {
	return strings.TrimSpace(cfg.Driver) != "" ||
		strings.TrimSpace(cfg.JSONL.Directory) != "" ||
		strings.TrimSpace(cfg.JSONL.Filename) != "" ||
		cfg.JSONL.SSH != nil ||
		strings.TrimSpace(cfg.MySQL.DSN) != "" ||
		strings.TrimSpace(cfg.MySQL.Table) != "" ||
		strings.TrimSpace(cfg.Redis.Address) != "" ||
		strings.TrimSpace(cfg.Redis.Password) != "" ||
		cfg.Redis.DB != 0 ||
		strings.TrimSpace(cfg.Redis.KeyPrefix) != ""
}

func joinRegistrationPath(prefix, child string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	child = strings.Trim(strings.TrimSpace(child), "/")
	if prefix == "" {
		return child
	}
	if child == "" {
		return prefix
	}
	return prefix + "/" + child
}

func isNilManagedFSM(machine RegisterableFSMI) bool {
	if machine == nil {
		return true
	}
	value := reflect.ValueOf(machine)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func listenerURL(listener net.Listener, endpoint string) string {
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "http://" + listener.Addr().String() + endpoint
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + endpoint
}
