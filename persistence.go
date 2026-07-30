package fsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swkings/fsm/store"
)

var (
	ErrInvalidStoreKey   = errors.New("fsm store key is empty")
	ErrStoreNotBound     = errors.New("fsm store is not bound")
	ErrPersistenceFailed = errors.New("fsm persistence failed")
	ErrRestoreName       = errors.New("stored fsm name does not match")
	ErrRestoreState      = errors.New("stored fsm state does not exist in current definition")
)

type persistenceBinding struct {
	sync.RWMutex
	backend store.StoreI
	key     string
}

// BindStore 将持久化后端绑定到 FSM，并按 key 恢复最近一次成功保存的数据。
// 找不到历史数据时会保存当前初始快照。key 应是业务实例的稳定标识。
func (fsm *FSM[T]) BindStore(ctx context.Context, backend store.StoreI, key string) error {
	if fsm == nil || backend == nil {
		return fmt.Errorf("%w: store is nil", ErrPersistenceFailed)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrInvalidStoreKey
	}
	if ctx == nil {
		ctx = context.Background()
	}

	fsm.eventLock.Lock()
	defer fsm.eventLock.Unlock()
	record, err := backend.Latest(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		fsm.bindStore(backend, key)
		if err = fsm.saveSnapshot(ctx, store.Trace{}, true); err != nil {
			fsm.UnbindStore()
		}

		return err
	}
	if err != nil {
		return fmt.Errorf("%w: load %q: %v", ErrPersistenceFailed, key, err)
	}
	if err = fsm.restoreRecord(record); err != nil {
		return err
	}
	fsm.bindStore(backend, key)
	fsm.notifyRuntimeObservers(nil)

	return nil
}

// UnbindStore 停止后续自动保存，但不会关闭由调用方持有的 Store。
func (fsm *FSM[T]) UnbindStore() {
	if fsm == nil {
		return
	}
	fsm.persistence.Lock()
	fsm.persistence.backend, fsm.persistence.key = nil, ""
	fsm.persistence.Unlock()
}

func (fsm *FSM[T]) bindStore(backend store.StoreI, key string) {
	fsm.persistence.Lock()
	fsm.persistence.backend, fsm.persistence.key = backend, key
	fsm.persistence.Unlock()
}

// Save 将 FSM 当前状态立即写入已绑定 Store，适用于 SetPayload、
// SetCurrentState 等非事件修改后的显式检查点。
func (fsm *FSM[T]) Save(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	fsm.persistence.RLock()
	bound := fsm.persistence.backend != nil
	fsm.persistence.RUnlock()
	if !bound {
		return ErrStoreNotBound
	}
	if !fsm.eventLock.TryLock() {
		return ErrorEventProcessing
	}
	defer fsm.eventLock.Unlock()

	return fsm.saveSnapshot(ctx, store.Trace{}, true)
}

func (fsm *FSM[T]) restoreRecord(record store.Record) error {
	metadata := record.Metadata
	if metadata.Name != "" && metadata.Name != string(fsm.Name) {
		return fmt.Errorf("%w: stored=%q current=%q", ErrRestoreName, metadata.Name, fsm.Name)
	}
	if metadata.CurrentState != "" && !fsm.knownState(FSMState(metadata.CurrentState)) {
		return fmt.Errorf("%w: %q", ErrRestoreState, metadata.CurrentState)
	}
	payload := fsm.Payload
	hasPayload := false
	if len(metadata.Payload) > 0 && string(metadata.Payload) != "null" {
		var target any = &payload
		if !isNilPayload(payload) {
			value := reflect.ValueOf(payload)
			if value.Kind() == reflect.Pointer {
				target = payload
			}
		}
		if err := json.Unmarshal(metadata.Payload, target); err != nil {
			return fmt.Errorf("%w: restore payload: %v", ErrPersistenceFailed, err)
		}
		hasPayload = true
	}
	fsm.stateLock.Lock()
	if metadata.InstanceID != "" {
		fsm.InstanceID = metadata.InstanceID
	}
	if metadata.TransitionID != "" {
		fsm.TtId = metadata.TransitionID
	}
	fsm.Describe = metadata.Description
	fsm.fromState = FSMState(metadata.PreviousState)
	fsm.fromEvent = FSMEvent(metadata.PreviousEvent)
	fsm.currentState = FSMState(metadata.CurrentState)
	fsm.processingEvent = EventEmpty
	fsm.processingContext = context.Background()
	if hasPayload {
		fsm.Payload = payload
	}
	fsm.stateLock.Unlock()

	return nil
}

func (fsm *FSM[T]) knownState(state FSMState) bool {
	if state == fsm.GetCurrentState() {
		return true
	}
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()
	for key, target := range fsm.transitions {
		if key.FromState == state || target == state {
			return true
		}
	}

	return false
}

func (fsm *FSM[T]) persistTrace(ctx context.Context, trace TransitionTrace) error {
	storeTrace := store.Trace{
		TransitionID: trace.TtId,
		PreviousID:   trace.PrevTtId,
		FromState:    string(trace.FromState),
		Event:        string(trace.Event),
		ToState:      string(trace.ToState),
		FinalState:   string(trace.FinalState),
		Success:      trace.TransitionResult,
		Entry:        metadataToStore(trace.EntryMetadata),
		Exit:         metadataToStore(trace.ExitMetadata),
		BeginTime:    trace.BeginTime,
		EndTime:      trace.EndTime,
	}
	if trace.Error != nil {
		storeTrace.Error = trace.Error.Error()
	}

	return fsm.saveSnapshot(ctx, storeTrace, trace.TransitionResult)
}

func (fsm *FSM[T]) saveSnapshot(ctx context.Context, trace store.Trace, success bool) error {
	fsm.persistence.RLock()
	backend, key := fsm.persistence.backend, fsm.persistence.key
	fsm.persistence.RUnlock()
	if backend == nil {
		return nil
	}
	metadata := fsm.Metadata()
	record := store.Record{
		ID:          uuid.NewString(),
		Key:         key,
		RecordedAt:  time.Now(),
		Success:     success,
		Metadata:    metadataToStore(metadata),
		Transitions: fsm.storeTransitions(),
		Trace:       trace,
	}
	if !isNilPayload(fsm.Payload) {
		record.DisplayName = fsm.Payload.DisplayName()
		record.Category = fsm.Payload.Category()
	}
	if trace.EndTime > 0 {
		record.RecordedAt = time.Unix(0, trace.EndTime)
	}
	if err := backend.Save(ctx, record); err != nil {
		return fmt.Errorf("%w: save %q: %v", ErrPersistenceFailed, key, err)
	}

	return nil
}

func (fsm *FSM[T]) storeTransitions() []store.Transition {
	fsm.configLock.RLock()
	transitions := make([]store.Transition, 0, len(fsm.transitions))
	for key, state := range fsm.transitions {
		transitions = append(transitions, store.Transition{FromState: string(key.FromState), Event: string(key.Event), ToState: string(state)})
	}
	fsm.configLock.RUnlock()
	sort.Slice(transitions, func(i, j int) bool {
		if transitions[i].FromState != transitions[j].FromState {
			return transitions[i].FromState < transitions[j].FromState
		}

		return transitions[i].Event < transitions[j].Event
	})

	return transitions
}

func metadataToStore(metadata FSMMetadata) store.Metadata {
	return store.Metadata{
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
