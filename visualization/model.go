// Package visualization 提供与具体状态机实现无关的运行时可视化协议和 Server。
//
// 第三方状态机只需直接实现 Client；需要主动推送更新时再实现 PushClient。
// 状态机定义在注册时一次性提交，后续 pull/push 只同步实时数据。
package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// Mode 表示 Server 获取客户端实时数据的方式。
type Mode string

const (
	// ModePull 由 Server 按 RegistrationConfig.PullInterval 周期调用客户端。
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

// RegistrationConfig 描述本次注册如何挂载和采集客户端。
// 配置属于 Registration 生命周期，不应保存在 FSM 客户端内部。
type RegistrationConfig struct {
	// DisplayName 是本次注册在 Web 侧展示的名称；为空时使用 Definition.Name。
	DisplayName string
	// Path 是本次注册所属目录，使用 / 分隔层级。
	Path string
	// Mode 指定 pull 或 push；为空时根据客户端是否实现 PushClient 自动判断。
	Mode Mode
	// PullInterval 是 pull 周期；ModePull 下为零时默认 5 秒。
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

// TransitionSnapshot 是一条与具体 FSM 类型无关的状态转移边。
type TransitionSnapshot struct {
	FromState string `json:"fromState"`
	Event     string `json:"event"`
	ToState   string `json:"toState"`
}

// Definition 是客户端注册时一次性提交的完整静态定义。
// 注册完成后 Server 不会再次调用 VisualizationDefinition。
type Definition struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"describe,omitempty"`
	States      []string             `json:"states"`
	Events      []string             `json:"events"`
	Transitions []TransitionSnapshot `json:"transitions"`
}

// Descriptor 返回 Definition 中稳定的实例描述。
func (definition Definition) Descriptor(config RegistrationConfig) Descriptor {
	return Descriptor{
		ID:          definition.ID,
		Name:        definition.Name,
		DisplayName: config.DisplayName,
		Description: definition.Description,
		Path:        config.Path,
	}
}

// MetadataSnapshot 是可视化协议中的 FSM 运行数据快照。
type MetadataSnapshot struct {
	InstanceID      string          `json:"instanceId"`
	TransitionID    string          `json:"transitionId"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	PreviousState   string          `json:"previousState"`
	PreviousEvent   string          `json:"previousEvent"`
	CurrentState    string          `json:"currentState"`
	ProcessingEvent string          `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    string          `json:"payloadError,omitempty"`
}

// TraceSnapshot 是一次状态转移执行结果的通用快照。
type TraceSnapshot struct {
	TransitionID  string           `json:"transitionId"`
	PreviousID    string           `json:"previousId"`
	FromState     string           `json:"fromState"`
	Event         string           `json:"event"`
	ToState       string           `json:"toState"`
	FinalState    string           `json:"finalState"`
	Success       bool             `json:"success"`
	Error         string           `json:"error,omitempty"`
	EntryMetadata MetadataSnapshot `json:"entryMetadata"`
	ExitMetadata  MetadataSnapshot `json:"exitMetadata"`
	BeginTime     int64            `json:"beginTime"`
	EndTime       int64            `json:"endTime"`
	Duration      time.Duration    `json:"duration"`
}

// RealtimeData 是注册完成后 pull/push 同步的易变数据。
// States、Events、Transitions 等静态定义不应出现在这里。
// Traces 表示客户端当前持有的完整 trace 快照；Server 会应用 TraceLimit。
type RealtimeData struct {
	TransitionID    string          `json:"transitionId"`
	CurrentState    string          `json:"currentState"`
	PreviousState   string          `json:"previousState"`
	PreviousEvent   string          `json:"previousEvent"`
	ProcessingEvent string          `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    error           `json:"payloadError,omitempty"`
	Traces          []TraceSnapshot `json:"traces,omitempty"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Snapshot 是 Server 合并静态 Definition 与 RealtimeData 后提供给 Web/API 的视图。
// 客户端不需要构造或同步该结构。
type Snapshot struct {
	Descriptor
	Mode                 Mode                 `json:"mode"`
	TransitionID         string               `json:"transitionId"`
	CurrentState         string               `json:"currentState"`
	PreviousState        string               `json:"previousState"`
	PreviousEvent        string               `json:"previousEvent"`
	ProcessingEvent      string               `json:"processingEvent,omitempty"`
	Payload              json.RawMessage      `json:"payload"`
	PayloadError         error                `json:"payloadError,omitempty"`
	CollectionError      string               `json:"collectionError,omitempty"`
	States               []string             `json:"states"`
	Events               []string             `json:"events"`
	Transitions          []TransitionSnapshot `json:"transitions"`
	AvailableTransitions []TransitionSnapshot `json:"availableTransitions"`
	Traces               []TraceSnapshot      `json:"traces"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

// Client 是 Server 定义的基础注册接口。
// VisualizationDefinition 仅在注册时调用一次；之后只调用 VisualizationSnapshot。
type Client interface {
	VisualizationDefinition() Definition
	VisualizationSnapshot(context.Context) (RealtimeData, error)
}

// PublishFunc 由 push 客户端提交最新实时数据。
// traces 是本次变化产生的增量 trace，Server 会追加并按 TraceLimit 截断。
type PublishFunc func(RealtimeData, ...TraceSnapshot) error

// PushClient 在 Client 基础上增加实时数据订阅。
type PushClient interface {
	Client
	SubscribeVisualization(PublishFunc) func()
}

func normalizeRegistration(
	definition Definition,
	config RegistrationConfig,
	supportsPush bool,
) (Definition, RegistrationConfig, error) {
	definition.ID = strings.TrimSpace(definition.ID)
	if definition.ID == "" || strings.ContainsAny(definition.ID, "/\\") {
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: %q", ErrInvalidRegistrationID, definition.ID)
	}
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Name == "" {
		definition.Name = definition.ID
	}
	definition.Description = strings.TrimSpace(definition.Description)

	config.DisplayName = strings.TrimSpace(config.DisplayName)
	if config.DisplayName == "" {
		config.DisplayName = definition.Name
	}
	path, err := NormalizePath(config.Path)
	if err != nil {
		return Definition{}, RegistrationConfig{}, err
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
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: unsupported mode %q", ErrRegistrationMode, config.Mode)
	}
	if config.Mode == ModePush && !supportsPush {
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: client does not implement PushClient", ErrInvalidClient)
	}
	if config.Mode == ModePull {
		if config.PullInterval < 0 {
			return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: pull interval must not be negative", ErrInvalidClient)
		}
		if config.PullInterval == 0 {
			config.PullInterval = 5 * time.Second
		}
	}
	completeDefinition(&definition)
	return definition, config, nil
}

func completeDefinition(definition *Definition) {
	stateSet := make(map[string]struct{}, len(definition.States))
	eventSet := make(map[string]struct{}, len(definition.Events))
	for _, state := range definition.States {
		if state = strings.TrimSpace(state); state != "" {
			stateSet[state] = struct{}{}
		}
	}
	for _, event := range definition.Events {
		if event = strings.TrimSpace(event); event != "" {
			eventSet[event] = struct{}{}
		}
	}
	for _, transition := range definition.Transitions {
		if transition.FromState != "" {
			stateSet[transition.FromState] = struct{}{}
		}
		if transition.ToState != "" {
			stateSet[transition.ToState] = struct{}{}
		}
		if transition.Event != "" {
			eventSet[transition.Event] = struct{}{}
		}
	}
	definition.States = sortedSet(stateSet)
	definition.Events = sortedSet(eventSet)
	definition.Transitions = append([]TransitionSnapshot(nil), definition.Transitions...)
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
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

func cloneDefinition(definition Definition) Definition {
	definition.States = append([]string(nil), definition.States...)
	definition.Events = append([]string(nil), definition.Events...)
	definition.Transitions = append([]TransitionSnapshot(nil), definition.Transitions...)
	return definition
}

func cloneTraceSnapshots(traces []TraceSnapshot) []TraceSnapshot {
	cloned := append([]TraceSnapshot(nil), traces...)
	for index := range cloned {
		cloned[index].EntryMetadata.Payload = append(json.RawMessage(nil), cloned[index].EntryMetadata.Payload...)
		cloned[index].ExitMetadata.Payload = append(json.RawMessage(nil), cloned[index].ExitMetadata.Payload...)
	}
	return cloned
}

func cloneRealtimeData(data RealtimeData) RealtimeData {
	data.Payload = append(json.RawMessage(nil), data.Payload...)
	data.Traces = cloneTraceSnapshots(data.Traces)
	return data
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Payload = append(json.RawMessage(nil), snapshot.Payload...)
	snapshot.States = append([]string(nil), snapshot.States...)
	snapshot.Events = append([]string(nil), snapshot.Events...)
	snapshot.Transitions = append([]TransitionSnapshot(nil), snapshot.Transitions...)
	snapshot.AvailableTransitions = append([]TransitionSnapshot(nil), snapshot.AvailableTransitions...)
	snapshot.Traces = cloneTraceSnapshots(snapshot.Traces)
	return snapshot
}
