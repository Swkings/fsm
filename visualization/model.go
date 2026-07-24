// Package visualization 提供与具体状态机实现无关的运行时可视化协议和 Server。
//
// 第三方状态机只需直接实现 Client；需要主动推送更新时再实现 PushClient。
// 注册元数据由 ClientConfig 一次性声明，客户端实现接口后即可直接注册。
package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrServerStarted 表示可视化 Server 已启动，不能重复启动。
	ErrServerStarted = errors.New("visualization server already started")
	// ErrServerClosed 表示可视化 Server 已关闭。
	ErrServerClosed = errors.New("visualization server is closed")
	// ErrInvalidClient 表示注册对象没有正确实现 Client。
	ErrInvalidClient = errors.New("invalid visualization client")
	// ErrInvalidRegistrationID 表示注册 ID 为空或包含路径分隔符。
	ErrInvalidRegistrationID = errors.New("invalid visualization registration id")
	// ErrRegistrationIDRepeated 表示注册 ID 已被其他实例占用。
	ErrRegistrationIDRepeated = errors.New("visualization registration id is repeated")
	// ErrInvalidPath 表示注册目录 path 不符合以 / 分层的规则。
	ErrInvalidPath = errors.New("invalid visualization registration path")
	// ErrRegistrationNotFound 表示指定注册不存在。
	ErrRegistrationNotFound = errors.New("visualization registration not found")
	// ErrRegistrationMode 表示在错误的采集模式下执行了操作。
	ErrRegistrationMode = errors.New("visualization registration mode mismatch")
)

// Mode 表示 Server 获取客户端更新的方式。
type Mode string

const (
	// ModePull 由 Server 按 ClientConfig.PullInterval 周期调用客户端。
	ModePull Mode = "pull"
	// ModePush 由客户端在状态变化时调用 Server 提供的 PublishFunc。
	ModePush Mode = "push"
)

// ServerConfig 是 Server 的全部启动配置。零值使用安全默认值。
type ServerConfig struct {
	// Address 是 Start 使用的监听地址，默认 127.0.0.1:0。
	Address string
	// TraceLimit 是每个实例保留的最近 trace 数量，默认 100。
	TraceLimit int
}

// ClientConfig 是客户端注册时一次性返回的元数据和工作模式。
type ClientConfig struct {
	ID           string
	Name         string
	DisplayName  string
	Description  string
	Path         string
	Mode         Mode
	PullInterval time.Duration
}

// Descriptor 描述一个已注册状态机实例，不包含运行时易变数据。
type Descriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"describe,omitempty"`
	// Path 使用 / 分隔目录层级，例如 production/order/checkout。
	Path string `json:"path,omitempty"`
}

// Descriptor 返回可写入 Snapshot 的稳定描述信息。
func (config ClientConfig) Descriptor() Descriptor {
	return Descriptor{
		ID:          config.ID,
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Description: config.Description,
		Path:        config.Path,
	}
}

// TransitionSnapshot 是一条与具体 FSM 类型无关的状态转移边。
type TransitionSnapshot struct {
	FromState string `json:"fromState"`
	Event     string `json:"event"`
	ToState   string `json:"toState"`
}

// TraceSnapshot 是一次状态转移执行结果的通用快照。
type TraceSnapshot struct {
	TransitionID string        `json:"transitionId"`
	PreviousID   string        `json:"previousId"`
	FromState    string        `json:"fromState"`
	Event        string        `json:"event"`
	ToState      string        `json:"toState"`
	FinalState   string        `json:"finalState"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
	BeginTime    int64         `json:"beginTime"`
	EndTime      int64         `json:"endTime"`
	Duration     time.Duration `json:"duration"`
}

// Snapshot 是 Web/API 使用的统一状态机视图。
// 客户端只需要填写运行时字段；Server 会覆盖 Descriptor、Mode 和 UpdatedAt 默认值。
type Snapshot struct {
	Descriptor
	Mode                 Mode                 `json:"mode"`
	TransitionID         string               `json:"transitionId"`
	CurrentState         string               `json:"currentState"`
	PreviousState        string               `json:"previousState"`
	PreviousEvent        string               `json:"previousEvent"`
	ProcessingEvent      string               `json:"processingEvent,omitempty"`
	Payload              json.RawMessage      `json:"payload"`
	PayloadError         string               `json:"payloadError,omitempty"`
	CollectionError      string               `json:"collectionError,omitempty"`
	States               []string             `json:"states"`
	Transitions          []TransitionSnapshot `json:"transitions"`
	AvailableTransitions []TransitionSnapshot `json:"availableTransitions"`
	Traces               []TraceSnapshot      `json:"traces"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

// Client 是 Server 定义的唯一基础注册接口。
// pull 客户端只需实现这两个方法，然后调用 server.Register(ctx, client)。
type Client interface {
	VisualizationConfig() ClientConfig
	VisualizationSnapshot(context.Context) (Snapshot, error)
}

// PublishFunc 通知 Server 客户端已经变化。Server 收到通知后会重新获取 Snapshot。
// traces 只传本次变化产生的增量记录。
type PublishFunc func(...TraceSnapshot) error

// PushClient 在 Client 基础上增加状态变化订阅。
// Server 注册时传入 PublishFunc，客户端返回取消订阅函数即可。
type PushClient interface {
	Client
	SubscribeVisualization(PublishFunc) func()
}

func normalizeClientConfig(config ClientConfig, supportsPush bool) (ClientConfig, error) {
	config.ID = strings.TrimSpace(config.ID)
	if config.ID == "" || strings.ContainsAny(config.ID, "/\\") {
		return ClientConfig{}, fmt.Errorf("%w: %q", ErrInvalidRegistrationID, config.ID)
	}
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		config.Name = config.ID
	}
	config.DisplayName = strings.TrimSpace(config.DisplayName)
	if config.DisplayName == "" {
		config.DisplayName = config.Name
	}
	config.Description = strings.TrimSpace(config.Description)
	path, err := NormalizePath(config.Path)
	if err != nil {
		return ClientConfig{}, err
	}
	config.Path = path
	if config.Mode == "" {
		if supportsPush {
			config.Mode = ModePush
		} else {
			config.Mode = ModePull
		}
	}
	if config.Mode != ModePull && config.Mode != ModePush {
		return ClientConfig{}, fmt.Errorf("%w: unsupported mode %q", ErrRegistrationMode, config.Mode)
	}
	if config.Mode == ModePush && !supportsPush {
		return ClientConfig{}, fmt.Errorf("%w: client does not implement PushClient", ErrInvalidClient)
	}
	if config.Mode == ModePull {
		if config.PullInterval < 0 {
			return ClientConfig{}, fmt.Errorf("%w: pull interval must not be negative", ErrInvalidClient)
		}
		if config.PullInterval == 0 {
			config.PullInterval = 5 * time.Second
		}
	}
	return config, nil
}

// NormalizePath 校验并规范化以 / 分层的注册路径。
func NormalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "", nil
	}
	if strings.Contains(path, "\\") {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
	}
	path = strings.Trim(path, "/")
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
		}
		segments[index] = segment
	}
	return strings.Join(segments, "/"), nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Payload = append(json.RawMessage(nil), snapshot.Payload...)
	snapshot.States = append([]string(nil), snapshot.States...)
	snapshot.Transitions = append([]TransitionSnapshot(nil), snapshot.Transitions...)
	snapshot.AvailableTransitions = append([]TransitionSnapshot(nil), snapshot.AvailableTransitions...)
	snapshot.Traces = append([]TraceSnapshot(nil), snapshot.Traces...)
	return snapshot
}
