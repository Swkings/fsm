package fsm

import (
	"fmt"

	"time"

	"github.com/swkings/fsm/common/util"
)

type TransitionTrace struct {
	TtId             string
	PrevTtId         string
	FromState        FSMState
	Event            FSMEvent
	ToState          FSMState
	TransitionResult bool
	FinalState       FSMState
	Error            error
	BeginTime        int64 // nanosecond timestamp
	EndTime          int64 // nanosecond timestamp
}

func NewTransitionTrace(prevTtId string, ttId string, fromState FSMState, event FSMEvent) *TransitionTrace {
	return &TransitionTrace{
		TtId:      ttId,
		PrevTtId:  prevTtId,
		FromState: fromState,
		Event:     event,
	}
}

func (tt *TransitionTrace) Start() *TransitionTrace {
	tt.BeginTime = time.Now().UnixNano()

	return tt
}

func (tt *TransitionTrace) End(errs ...error) {
	if len(errs) > 0 {
		tt.Error = errs[0]
	}
	tt.EndTime = time.Now().UnixNano()
}

func (tt *TransitionTrace) ReleaseControl() *TransitionTrace {
	return tt
}

func (tt *TransitionTrace) SetToState(toState FSMState) *TransitionTrace {
	tt.ToState = toState

	return tt
}

func (tt *TransitionTrace) SetFinalState(finalState FSMState) *TransitionTrace {
	tt.FinalState = finalState

	return tt
}

func (tt *TransitionTrace) TransitionSuccess() *TransitionTrace {
	tt.TransitionResult = true

	return tt
}

func (tt *TransitionTrace) TransitionFail() *TransitionTrace {
	tt.TransitionResult = false

	return tt
}

func (tt *TransitionTrace) TrimPtr() TransitionTrace {
	return *tt
}

func (tt *TransitionTrace) String() string {
	return fmt.Sprintf("%v", util.PrettyMapStruct(tt, false))
}
