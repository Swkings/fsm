package fsm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swkings/fsm/visualization"
)

type fsmVisualizationConfig struct {
	id          string
	name        string
	displayName string
	description string
	path        string
}

// SetVisualizationConfig 设置当前 FSM 注册到可视化 Server 时使用的元数据。
// FSM 固定采用 push 模式，因此会忽略 config.Mode 和 config.PullInterval。
func (fsm *FSM[T]) SetVisualizationConfig(config visualization.ClientConfig) *FSM[T] {
	if fsm == nil {
		return fsm
	}
	fsm.visualizationClientLock.Lock()
	fsm.visualizationClientConfig = fsmVisualizationConfig{
		id:          config.ID,
		name:        config.Name,
		displayName: config.DisplayName,
		description: config.Description,
		path:        config.Path,
	}
	fsm.visualizationClientLock.Unlock()
	return fsm
}

// VisualizationConfig 使 FSM 直接实现 visualization.Client。
// 未显式设置的字段会使用 FSM 自身的实例 ID、名称和描述。
func (fsm *FSM[T]) VisualizationConfig() visualization.ClientConfig {
	if fsm == nil {
		return visualization.ClientConfig{}
	}
	fsm.visualizationClientLock.RLock()
	config := fsm.visualizationClientConfig
	fsm.visualizationClientLock.RUnlock()

	fsm.stateLock.RLock()
	defaultID := fsm.instanceID
	defaultName := string(fsm.Name)
	defaultDescription := fsm.Describe
	fsm.stateLock.RUnlock()
	if config.id == "" {
		config.id = defaultID
	}
	if config.name == "" {
		config.name = defaultName
	}
	if config.displayName == "" {
		config.displayName = config.name
	}
	if config.description == "" {
		config.description = defaultDescription
	}
	return visualization.ClientConfig{
		ID:          config.id,
		Name:        config.name,
		DisplayName: config.displayName,
		Description: config.description,
		Path:        config.path,
		Mode:        visualization.ModePush,
	}
}

// VisualizationSnapshot 返回 Server 所需的通用运行时快照。
func (fsm *FSM[T]) VisualizationSnapshot(ctx context.Context) (visualization.Snapshot, error) {
	if fsm == nil {
		return visualization.Snapshot{}, visualization.ErrInvalidClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return visualization.Snapshot{}, err
	}
	return fsm.runtimeVisualizationSnapshot(), nil
}

// SubscribeVisualization 使 FSM 直接实现 visualization.PushClient。
// 每次事件结束时只发布增量 trace，Server 会自行重新读取最新 Snapshot。
func (fsm *FSM[T]) SubscribeVisualization(publish visualization.PublishFunc) func() {
	if fsm == nil || publish == nil {
		return func() {}
	}
	observerID := uuid.NewString()
	return fsm.addRuntimeObserver(observerID, func(trace *TransitionTrace) {
		_ = publish(newVisualizationTraceSnapshot(*trace))
	})
}

func (fsm *FSM[T]) runtimeVisualizationSnapshot() visualization.Snapshot {
	fsm.stateLock.RLock()
	transitionID := fsm.TtId
	currentState := string(fsm.currentState)
	previousState := string(fsm.fromState)
	previousEvent := string(fsm.fromEvent)
	processingEvent := string(fsm.processingEvent)
	payload := fsm.Payload
	fsm.stateLock.RUnlock()

	fsm.configLock.RLock()
	transitions := make(map[EventStateKey]FSMState, len(fsm.transitions))
	for key, target := range fsm.transitions {
		transitions[key] = target
	}
	fsm.configLock.RUnlock()
	keys := getSortedTransitionKeys(transitions)
	transitionSnapshots := make([]visualization.TransitionSnapshot, 0, len(keys))
	availableTransitions := make([]visualization.TransitionSnapshot, 0)
	stateSet := map[string]struct{}{currentState: {}}
	for _, key := range keys {
		transition := visualization.TransitionSnapshot{
			FromState: string(key.FromState),
			Event:     string(key.Event),
			ToState:   string(transitions[key]),
		}
		transitionSnapshots = append(transitionSnapshots, transition)
		stateSet[transition.FromState] = struct{}{}
		stateSet[transition.ToState] = struct{}{}
		if transition.FromState == currentState {
			availableTransitions = append(availableTransitions, transition)
		}
	}
	states := make([]string, 0, len(stateSet))
	for state := range stateSet {
		states = append(states, state)
	}
	sort.Strings(states)
	payloadJSON, payloadError := marshalFSMPayload(payload)
	return visualization.Snapshot{
		TransitionID:         transitionID,
		CurrentState:         currentState,
		PreviousState:        previousState,
		PreviousEvent:        previousEvent,
		ProcessingEvent:      processingEvent,
		Payload:              payloadJSON,
		PayloadError:         payloadError,
		States:               states,
		Transitions:          transitionSnapshots,
		AvailableTransitions: availableTransitions,
		UpdatedAt:            time.Now(),
	}
}

func marshalFSMPayload(value any) (payload json.RawMessage, payloadError string) {
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

func newVisualizationTraceSnapshot(trace TransitionTrace) visualization.TraceSnapshot {
	errorMessage := ""
	if trace.Error != nil {
		errorMessage = trace.Error.Error()
	}
	return visualization.TraceSnapshot{
		TransitionID: trace.TtId,
		PreviousID:   trace.PrevTtId,
		FromState:    string(trace.FromState),
		Event:        string(trace.Event),
		ToState:      string(trace.ToState),
		FinalState:   string(trace.FinalState),
		Success:      trace.TransitionResult,
		Error:        errorMessage,
		BeginTime:    trace.BeginTime,
		EndTime:      trace.EndTime,
		Duration:     trace.Duration(),
	}
}

func (fsm *FSM[T]) addRuntimeObserver(id string, observer func(*TransitionTrace)) func() {
	fsm.runtimeObserverLock.Lock()
	fsm.runtimeObservers[id] = observer
	fsm.runtimeObserverLock.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			fsm.runtimeObserverLock.Lock()
			delete(fsm.runtimeObservers, id)
			fsm.runtimeObserverLock.Unlock()
		})
	}
}

func (fsm *FSM[T]) hasRuntimeObservers() bool {
	fsm.runtimeObserverLock.RLock()
	defer fsm.runtimeObserverLock.RUnlock()
	return len(fsm.runtimeObservers) > 0
}

func (fsm *FSM[T]) rejectedRuntimeEventTrace(event FSMEvent, err error) TransitionTrace {
	trace := fsm.rejectedEventTrace(event, err)
	fsm.notifyRuntimeObservers(&trace)
	return trace
}

func (fsm *FSM[T]) notifyRuntimeObservers(trace *TransitionTrace) {
	fsm.runtimeObserverLock.RLock()
	observers := make([]func(*TransitionTrace), 0, len(fsm.runtimeObservers))
	for _, observer := range fsm.runtimeObservers {
		observers = append(observers, observer)
	}
	fsm.runtimeObserverLock.RUnlock()
	for _, observer := range observers {
		func() {
			defer func() { _ = recover() }()
			observer(trace)
		}()
	}
}
