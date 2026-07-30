package fsm

// SetTransitions validates and atomically registers transition rules.
func (fsm *FSM[T]) SetTransitions(items ...TransitionItem[T]) *FSM[T] {
	fsm.configLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.configLock.Unlock()

	seen := make(map[EventStateKey]struct{}, len(items))
	for _, item := range items {
		key := EventStateKey{Event: item.Event, FromState: item.FromState}
		if _, exists := fsm.transitions[key]; exists {
			panic(ErrorEventStateKeyRepeated(key))
		}
		if _, repeated := seen[key]; repeated {
			panic(ErrorEventStateKeyRepeated(key))
		}
		seen[key] = struct{}{}
	}
	for _, item := range items {
		key := EventStateKey{Event: item.Event, FromState: item.FromState}
		fsm.transitions[key] = item.ToState
		fsm.handlers[key] = item.Handler
		fsm.setEventSearch(key)
	}

	return fsm
}

func (fsm *FSM[T]) setEventSearch(key EventStateKey) {
	if _, ok := fsm.eventSearch[key.FromState]; !ok {
		fsm.eventSearch[key.FromState] = []FSMEvent{}
	}
	fsm.eventSearch[key.FromState] = append(fsm.eventSearch[key.FromState], key.Event)
}

// GetEventUnderState returns a copy of the events accepted by state.
func (fsm *FSM[T]) GetEventUnderState(state FSMState) []FSMEvent {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()

	return append([]FSMEvent(nil), fsm.eventSearch[state]...)
}

func (fsm *FSM[T]) getTransition(key EventStateKey) (FSMState, Handler[T], bool) {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()
	state, exists := fsm.transitions[key]

	return state, fsm.handlers[key], exists
}

func (fsm *FSM[T]) visualizationSnapshot() (map[EventStateKey]FSMState, FSMState) {
	fsm.configLock.RLock()
	transitions := make(map[EventStateKey]FSMState, len(fsm.transitions))
	for key, state := range fsm.transitions {
		transitions[key] = state
	}
	fsm.configLock.RUnlock()
	currentState, _ := fsm.stateSnapshot()

	return transitions, currentState
}
