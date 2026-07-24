package fsm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swkings/fsm/visualization"
)

// RegisterVisualization 以 push 模式把当前 FSM 注册到任意 visualization.Registrar。
// 返回的 Registration 可用于动态注销；Server 关闭时注册的运行时观察器会自动解除。
func (fsm *FSM[T]) RegisterVisualization(
	ctx context.Context,
	registrar visualization.Registrar,
	options ...visualization.RegistrationOption,
) (visualization.Registration, error) {
	if fsm == nil || isNilVisualizationRegistrar(registrar) {
		return nil, visualization.ErrInvalidSource
	}
	if ctx == nil {
		ctx = context.Background()
	}
	descriptor := visualization.Descriptor{
		ID:          fsm.instanceID,
		Name:        string(fsm.Name),
		DisplayName: string(fsm.Name),
		Description: fsm.Describe,
	}
	pusher, err := registrar.RegisterPush(ctx, descriptor, options...)
	if err != nil {
		return nil, err
	}
	publisher := &fsmVisualizationPublisher[T]{fsm: fsm, pusher: pusher}
	observerID := uuid.NewString()
	unsubscribe := fsm.addRuntimeObserver(observerID, func(trace *TransitionTrace) {
		publisher.publish(trace)
	})
	registration := &fsmVisualizationRegistration{
		Pusher:      pusher,
		unsubscribe: unsubscribe,
	}
	if err := publisher.push(ctx, nil); err != nil {
		registration.Unregister()
		return nil, err
	}
	go registration.releaseObserverWhenDone()
	return registration, nil
}

// Register 是忽略注册错误的链式便捷方法。
// 需要处理错误或保存注销句柄时，请调用 RegisterVisualization。
func (fsm *FSM[T]) Register(
	registrar visualization.Registrar,
	options ...visualization.RegistrationOption,
) *FSM[T] {
	if fsm == nil || isNilVisualizationRegistrar(registrar) {
		return fsm
	}
	_, _ = fsm.RegisterVisualization(context.Background(), registrar, options...)
	return fsm
}

// FSMVisualizable 把 FSM 和注册选项包装成 visualization.Registrable，
// 适合通过 server.Register(ctx, target) 使用对象自注册协议。
func FSMVisualizable[T any](
	fsm *FSM[T],
	options ...visualization.RegistrationOption,
) visualization.Registrable {
	return fsmRegistrable[T]{
		fsm:     fsm,
		options: append([]visualization.RegistrationOption(nil), options...),
	}
}

type fsmRegistrable[T any] struct {
	fsm     *FSM[T]
	options []visualization.RegistrationOption
}

func (target fsmRegistrable[T]) RegisterVisualization(
	ctx context.Context,
	registrar visualization.Registrar,
) (visualization.Registration, error) {
	return target.fsm.RegisterVisualization(ctx, registrar, target.options...)
}

type fsmVisualizationPublisher[T any] struct {
	mu     sync.Mutex
	fsm    *FSM[T]
	pusher visualization.Pusher
}

func (publisher *fsmVisualizationPublisher[T]) publish(trace *TransitionTrace) {
	_ = publisher.push(context.Background(), trace)
}

func (publisher *fsmVisualizationPublisher[T]) push(ctx context.Context, trace *TransitionTrace) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	snapshot := publisher.fsm.runtimeVisualizationSnapshot()
	if trace == nil {
		return publisher.pusher.Push(ctx, snapshot)
	}
	return publisher.pusher.Push(ctx, snapshot, newVisualizationTraceSnapshot(*trace))
}

type fsmVisualizationRegistration struct {
	visualization.Pusher
	once        sync.Once
	unsubscribe func()
}

func (registration *fsmVisualizationRegistration) Unregister() bool {
	registration.releaseObserver()
	return registration.Pusher.Unregister()
}

func (registration *fsmVisualizationRegistration) releaseObserverWhenDone() {
	<-registration.Done()
	registration.releaseObserver()
}

func (registration *fsmVisualizationRegistration) releaseObserver() {
	registration.once.Do(func() {
		if registration.unsubscribe != nil {
			registration.unsubscribe()
		}
	})
}

func (fsm *FSM[T]) runtimeVisualizationSnapshot() visualization.Snapshot {
	fsm.stateLock.RLock()
	descriptor := visualization.Descriptor{
		ID:          fsm.instanceID,
		Name:        string(fsm.Name),
		DisplayName: string(fsm.Name),
		Description: fsm.Describe,
	}
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
		Descriptor:           descriptor,
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

func isNilVisualizationRegistrar(registrar visualization.Registrar) bool {
	if registrar == nil {
		return true
	}
	value := reflect.ValueOf(registrar)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
