package visualization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/swkings/fsm/config"
)

var ErrClientServerNotConfigured = errors.New("visualization client server is not configured")

// VisualizationConfig 配置业务进程内的全局单例可视化 Server。
// PathPrefix 会与各 ClientI 提供的 Path 组合；两者都允许为空。
type VisualizationConfig = config.Visualization

// RegistrationConfigProviderI 允许 ClientI 自行提供显示名称、目录和采集模式。
// fsm.FSM 会从 Payload 的 DisplayName() 与 Category() 方法生成该配置。
type RegistrationConfigProviderI interface {
	VisualizationRegistrationConfig() RegistrationConfig
}

var processClientServer struct {
	sync.RWMutex
	server *Server
}

func newClientServer(config VisualizationConfig) *Server {
	processClientServer.Lock()
	defer processClientServer.Unlock()
	if processClientServer.server != nil {
		return processClientServer.server
	}
	server := NewServerWithConfig(ServerConfig{
		Address:    config.ServerAddress,
		TraceLimit: config.TraceLimit,
	})
	server.pathPrefix = strings.TrimSpace(config.PathPrefix)
	processClientServer.server = server
	return server
}

// ClientServer 返回当前进程通过 NewServer(config) 创建的全局可视化 Server。
func ClientServer() (*Server, bool) {
	processClientServer.RLock()
	defer processClientServer.RUnlock()
	return processClientServer.server, processClientServer.server != nil
}

// RegisterClient 将 ClientI 注册到进程内全局 Server。
// 注册元数据由可选的 RegistrationConfigProviderI 提供，不再由调用方传入。
func RegisterClient(ctx context.Context, client ClientI) (*Registration, error) {
	server, exists := ClientServer()
	if !exists {
		return nil, ErrClientServerNotConfigured
	}
	config, err := registrationConfigFromClient(client)
	if err != nil {
		return nil, err
	}
	config.Path = joinClientPath(server.pathPrefix, config.Path)
	return server.Register(ctx, client, config)
}

func registrationConfigFromClient(client ClientI) (config RegistrationConfig, err error) {
	if isNilInterface(client) {
		return RegistrationConfig{}, ErrInvalidClient
	}
	provider, exists := client.(RegistrationConfigProviderI)
	if !exists {
		return RegistrationConfig{}, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: registration config panic: %v", ErrInvalidClient, recovered)
		}
	}()
	return provider.VisualizationRegistrationConfig(), nil
}

func joinClientPath(prefix, path string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	path = strings.Trim(strings.TrimSpace(path), "/")
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	return prefix + "/" + path
}

// ClientServerURL 返回当前进程的可视化前端地址。
func ClientServerURL() string {
	server, exists := ClientServer()
	if !exists {
		return ""
	}
	return server.URL()
}

// ShutdownClientServer 关闭并清理进程内全局 Server，关闭后可以重新创建。
func ShutdownClientServer(ctx context.Context) error {
	processClientServer.Lock()
	defer processClientServer.Unlock()
	server := processClientServer.server
	if server == nil {
		return nil
	}
	err := server.Close(ctx)
	processClientServer.server = nil
	return err
}
