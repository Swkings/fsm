package fsm

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swkings/fsm/visualization"
)

// VisualizationDefinition 返回完整静态定义，使 FSM 直接实现 visualization.Client。
// Server 仅在注册时调用一次；注册后修改转移配置不会自动改变已注册的状态图。
func (fsm *FSM[T]) VisualizationDefinition() visualization.Definition {
	if fsm == nil {
		return visualization.Definition{}
	}
	fsm.stateLock.RLock()
	id := fsm.InstanceID
	name := string(fsm.Name)
	description := fsm.Describe
	currentState := string(fsm.currentState)
	fsm.stateLock.RUnlock()

	fsm.configLock.RLock()
	transitions := make(map[EventStateKey]FSMState, len(fsm.transitions))
	for key, target := range fsm.transitions {
		transitions[key] = target
	}
	fsm.configLock.RUnlock()

	keys := getSortedTransitionKeys(transitions)
	transitionSnapshots := make([]visualization.TransitionSnapshot, 0, len(keys))
	stateSet := map[string]struct{}{currentState: {}}
	eventSet := make(map[string]struct{})
	for _, key := range keys {
		transition := visualization.TransitionSnapshot{
			FromState: string(key.FromState),
			Event:     string(key.Event),
			ToState:   string(transitions[key]),
		}
		transitionSnapshots = append(transitionSnapshots, transition)
		stateSet[transition.FromState] = struct{}{}
		stateSet[transition.ToState] = struct{}{}
		eventSet[transition.Event] = struct{}{}
	}
	states := make([]string, 0, len(stateSet))
	for state := range stateSet {
		states = append(states, state)
	}
	sort.Strings(states)
	events := make([]string, 0, len(eventSet))
	for event := range eventSet {
		events = append(events, event)
	}
	sort.Strings(events)

	return visualization.Definition{
		ID:          id,
		Name:        name,
		Description: description,
		States:      states,
		Events:      events,
		Transitions: transitionSnapshots,
	}
}

// VisualizationRegistrationConfig 从 Payload 获取可视化显示名称与相对目录。
// Payload 的 DisplayName() 与 Category() 返回空字符串时表示不设置。
func (fsm *FSM[T]) VisualizationRegistrationConfig() visualization.RegistrationConfig {
	if fsm == nil {
		return visualization.RegistrationConfig{}
	}
	fsm.stateLock.RLock()
	payload := fsm.Payload
	fsm.stateLock.RUnlock()
	if isNilPayload(payload) {
		return visualization.RegistrationConfig{}
	}
	return visualization.RegistrationConfig{
		DisplayName: payload.DisplayName(),
		Path:        payload.Category(),
	}
}

// VisualizationSnapshot 只返回易变的实时数据，不再重复构造状态图。
func (fsm *FSM[T]) VisualizationSnapshot(ctx context.Context) (visualization.RealtimeData, error) {
	if fsm == nil {
		return visualization.RealtimeData{}, visualization.ErrInvalidClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return visualization.RealtimeData{}, err
	}
	return fsm.runtimeVisualizationData(), nil
}

// SubscribeVisualization 使 FSM 直接实现 visualization.PushClient。
// 每次事件结束时直接推送最新实时数据及本次增量 trace。
func (fsm *FSM[T]) SubscribeVisualization(publish visualization.PublishFunc) func() {
	if fsm == nil || publish == nil {
		return func() {}
	}
	observerID := uuid.NewString()
	return fsm.addRuntimeObserver(observerID, func(trace *TransitionTrace) {
		data := fsm.runtimeVisualizationData()
		if trace == nil {
			_ = publish(data)
			return
		}
		_ = publish(data, newVisualizationTraceSnapshot(*trace))
	})
}

func (fsm *FSM[T]) runtimeVisualizationData() visualization.RealtimeData {
	metadata := fsm.Metadata()
	var payloadError error
	if metadata.PayloadError != "" {
		payloadError = errors.New(metadata.PayloadError)
	}
	return visualization.RealtimeData{
		TransitionID:    metadata.TransitionID,
		CurrentState:    string(metadata.CurrentState),
		PreviousState:   string(metadata.PreviousState),
		PreviousEvent:   string(metadata.PreviousEvent),
		ProcessingEvent: string(metadata.ProcessingEvent),
		Payload:         append(json.RawMessage(nil), metadata.Payload...),
		PayloadError:    payloadError,
		UpdatedAt:       time.Now(),
	}
}

func newVisualizationMetadataSnapshot(metadata FSMMetadata) visualization.MetadataSnapshot {
	return visualization.MetadataSnapshot{
		InstanceID:      metadata.InstanceID,
		TransitionID:    metadata.TransitionID,
		Name:            string(metadata.Name),
		Description:     metadata.Description,
		PreviousState:   string(metadata.PreviousState),
		PreviousEvent:   string(metadata.PreviousEvent),
		CurrentState:    string(metadata.CurrentState),
		ProcessingEvent: string(metadata.ProcessingEvent),
		Payload:         append(json.RawMessage(nil), metadata.Payload...),
		PayloadError:    metadata.PayloadError,
	}
}

func newVisualizationTraceSnapshot(trace TransitionTrace) visualization.TraceSnapshot {
	errorMessage := ""
	if trace.Error != nil {
		errorMessage = trace.Error.Error()
	}
	return visualization.TraceSnapshot{
		TransitionID:  trace.TtId,
		PreviousID:    trace.PrevTtId,
		FromState:     string(trace.FromState),
		Event:         string(trace.Event),
		ToState:       string(trace.ToState),
		FinalState:    string(trace.FinalState),
		Success:       trace.TransitionResult,
		Error:         errorMessage,
		EntryMetadata: newVisualizationMetadataSnapshot(trace.EntryMetadata),
		ExitMetadata:  newVisualizationMetadataSnapshot(trace.ExitMetadata),
		BeginTime:     trace.BeginTime,
		EndTime:       trace.EndTime,
		Duration:      trace.Duration(),
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
