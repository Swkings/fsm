// Package visualization 提供与具体状态机实现无关的运行时可视化协议和 Server。
//
// 第三方状态机只需实现 PullSource 或 Registrable，无需依赖 github.com/swkings/fsm
// 的泛型类型。客户端也可以通过 RegisterPush 获得 Pusher，主动提交快照。
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
	// ErrInvalidSource 表示 pull source 或自注册对象无效。
	ErrInvalidSource = errors.New("invalid visualization source")
	// ErrInvalidRegistrationID 表示注册 ID 为空或包含路径分隔符。
	ErrInvalidRegistrationID = errors.New("invalid visualization registration id")
	// ErrRegistrationIDRepeated 表示注册 ID 已被其他实例占用。
	ErrRegistrationIDRepeated = errors.New("visualization registration id is repeated")
	// ErrInvalidPath 表示注册目录 path 不符合以 / 分层的规则。
	ErrInvalidPath = errors.New("invalid visualization registration path")
	// ErrNilPayloadProvider 表示自定义 Payload 快照函数为 nil。
	ErrNilPayloadProvider = errors.New("visualization payload provider is nil")
	// ErrRegistrationNotFound 表示指定注册不存在。
	ErrRegistrationNotFound = errors.New("visualization registration not found")
	// ErrRegistrationMode 表示在错误的采集模式下执行了操作。
	ErrRegistrationMode = errors.New("visualization registration mode mismatch")
)

// Mode 表示 Server 获取状态机数据的方式。
type Mode string

const (
	// ModePull 由 Server 按配置周期主动调用 PullSource.Collect。
	ModePull Mode = "pull"
	// ModePush 由客户端通过 Pusher.Push 主动提交快照。
	ModePush Mode = "push"
)

// Descriptor 描述一个可注册的状态机实例，不包含运行时易变数据。
type Descriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"describe,omitempty"`
	// Path 使用 / 分隔目录层级，例如 production/order/checkout。
	Path string `json:"path,omitempty"`
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
// PullSource 和 push 客户端都通过该结构与 Server 交换数据。
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

// PullSource 是 pull 模式的数据源。Descriptor 必须稳定，Collect 应尊重 ctx 取消。
type PullSource interface {
	Descriptor() Descriptor
	Collect(context.Context) (Snapshot, error)
}

// PullSourceFunc 便于用函数快速接入 pull 模式。
type PullSourceFunc struct {
	Meta        Descriptor
	CollectFunc func(context.Context) (Snapshot, error)
}

// Descriptor 返回函数数据源的稳定描述信息。
func (source PullSourceFunc) Descriptor() Descriptor { return source.Meta }

// Collect 调用 CollectFunc 获取快照。
func (source PullSourceFunc) Collect(ctx context.Context) (Snapshot, error) {
	if source.CollectFunc == nil {
		return Snapshot{}, ErrInvalidSource
	}
	return source.CollectFunc(ctx)
}

// Registration 表示一个活动注册，可用于获取 ID、模式、生命周期或动态注销。
type Registration interface {
	ID() string
	Mode() Mode
	Done() <-chan struct{}
	Unregister() bool
}

// Pusher 是 push 模式注册句柄。traces 参数是本次更新产生的增量 trace；
// Snapshot.Traces 非 nil 时则视为客户端提供的完整 trace 历史。
type Pusher interface {
	Registration
	Push(context.Context, Snapshot, ...TraceSnapshot) error
}

// Registrar 是第三方接入只需依赖的注册接口。
// 自定义 Server 或测试替身也可以实现该接口。
type Registrar interface {
	RegisterPull(context.Context, PullSource, ...RegistrationOption) (Registration, error)
	RegisterPush(context.Context, Descriptor, ...RegistrationOption) (Pusher, error)
}

// Registrable 让一个类型拥有自己的注册方式。Server.Register 会回调该方法，
// 实现方可在内部自行选择 pull 或 push 模式及注册配置。
type Registrable interface {
	RegisterVisualization(context.Context, Registrar) (Registration, error)
}

// RegistrationOption 配置单个注册实例。
type RegistrationOption func(*registrationConfig) error

type registrationConfig struct {
	descriptor      Descriptor
	pullInterval    time.Duration
	payloadProvider func() any
}

func defaultRegistrationConfig(descriptor Descriptor) registrationConfig {
	return registrationConfig{descriptor: descriptor, pullInterval: 5 * time.Second}
}

// WithRegistrationID 覆盖数据源声明的稳定注册 ID。
func WithRegistrationID(id string) RegistrationOption {
	return func(config *registrationConfig) error {
		config.descriptor.ID = id
		return nil
	}
}

// WithDisplayName 设置 Web 页面显示名称。
func WithDisplayName(name string) RegistrationOption {
	return func(config *registrationConfig) error {
		config.descriptor.DisplayName = strings.TrimSpace(name)
		return nil
	}
}

// WithDescription 设置实例说明。
func WithDescription(description string) RegistrationOption {
	return func(config *registrationConfig) error {
		config.descriptor.Description = strings.TrimSpace(description)
		return nil
	}
}

// WithPath 设置多级目录路径，使用 / 分隔，例如 production/order/checkout。
func WithPath(path string) RegistrationOption {
	return func(config *registrationConfig) error {
		config.descriptor.Path = path
		return nil
	}
}

// WithPullInterval 设置 pull 模式采集间隔。默认 5 秒。
func WithPullInterval(interval time.Duration) RegistrationOption {
	return func(config *registrationConfig) error {
		if interval <= 0 {
			return fmt.Errorf("%w: pull interval must be positive", ErrInvalidSource)
		}
		config.pullInterval = interval
		return nil
	}
}

// WithPayloadProvider 设置统一的 Payload 快照函数，可用于脱敏或处理无法 JSON 编码的数据。
func WithPayloadProvider(provider func() any) RegistrationOption {
	return func(config *registrationConfig) error {
		if provider == nil {
			return ErrNilPayloadProvider
		}
		config.payloadProvider = provider
		return nil
	}
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

func applyRegistrationOptions(descriptor Descriptor, options []RegistrationOption) (registrationConfig, error) {
	config := defaultRegistrationConfig(descriptor)
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&config); err != nil {
			return registrationConfig{}, err
		}
	}
	config.descriptor.ID = strings.TrimSpace(config.descriptor.ID)
	if config.descriptor.ID == "" || strings.ContainsAny(config.descriptor.ID, "/\\") {
		return registrationConfig{}, fmt.Errorf("%w: %q", ErrInvalidRegistrationID, config.descriptor.ID)
	}
	config.descriptor.Name = strings.TrimSpace(config.descriptor.Name)
	if config.descriptor.Name == "" {
		config.descriptor.Name = config.descriptor.ID
	}
	config.descriptor.DisplayName = strings.TrimSpace(config.descriptor.DisplayName)
	if config.descriptor.DisplayName == "" {
		config.descriptor.DisplayName = config.descriptor.Name
	}
	path, err := NormalizePath(config.descriptor.Path)
	if err != nil {
		return registrationConfig{}, err
	}
	config.descriptor.Path = path
	return config, nil
}

func marshalPayload(value any) (payload json.RawMessage, payloadError string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			payloadError = "payload snapshot panic: " + fmt.Sprint(recovered)
			payload, _ = json.Marshal(map[string]string{"error": payloadError})
		}
	}()
	data, err := json.Marshal(value)
	if err == nil {
		return data, ""
	}
	fallback, _ := json.Marshal(map[string]string{
		"error": err.Error(),
		"value": fmt.Sprintf("%+v", value),
	})
	return fallback, err.Error()
}

func snapshotPayload(provider func() any) (payload json.RawMessage, payloadError string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			payloadError = "payload provider panic: " + fmt.Sprint(recovered)
			payload, _ = json.Marshal(map[string]string{"error": payloadError})
		}
	}()
	return marshalPayload(provider())
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Payload = append(json.RawMessage(nil), snapshot.Payload...)
	snapshot.States = append([]string(nil), snapshot.States...)
	snapshot.Transitions = append([]TransitionSnapshot(nil), snapshot.Transitions...)
	snapshot.AvailableTransitions = append([]TransitionSnapshot(nil), snapshot.AvailableTransitions...)
	snapshot.Traces = append([]TraceSnapshot(nil), snapshot.Traces...)
	return snapshot
}
