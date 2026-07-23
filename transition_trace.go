package fsm

import (
	"fmt"

	"time"

	"github.com/swkings/fsm/common/util"
)

// TransitionTrace 记录一次 ProcessEvent 调用从开始到结束的迁移信息。
// 无论迁移成功还是失败，调用方都可以通过它还原事件、状态和耗时。
type TransitionTrace struct {
	// TtId 是本次迁移预生成的追踪标识。
	TtId string
	// PrevTtId 是迁移开始前 FSM 持有的追踪标识，用于串联相邻迁移。
	PrevTtId string
	// FromState 是处理事件前的状态。
	FromState FSMState
	// Event 是本次尝试处理的事件。
	Event FSMEvent
	// ToState 是迁移规则声明的目标状态；规则不存在时保持零值。
	ToState FSMState
	// TransitionResult 表示目标状态是否成功提交。
	TransitionResult bool
	// FinalState 是本次处理结束时状态机实际所处的状态。
	FinalState FSMState
	// Error 保存迁移失败的原因；成功时为 nil。
	Error error
	// BeginTime 是迁移开始时的 Unix 纳秒时间戳。
	BeginTime int64
	// EndTime 是迁移结束时的 Unix 纳秒时间戳。
	EndTime int64
}

// NewTransitionTrace 创建迁移追踪对象。调用 Start 后才会记录开始时间。
func NewTransitionTrace(prevTtId string, ttId string, fromState FSMState, event FSMEvent) *TransitionTrace {
	return &TransitionTrace{
		TtId:      ttId,
		PrevTtId:  prevTtId,
		FromState: fromState,
		Event:     event,
	}
}

// Start 记录当前 Unix 纳秒时间为迁移开始时间，并返回自身以支持链式调用。
func (tt *TransitionTrace) Start() *TransitionTrace {
	tt.BeginTime = time.Now().UnixNano()

	return tt
}

// End 记录迁移结束时间；传入多个错误时只保存第一个错误。
func (tt *TransitionTrace) End(errs ...error) {
	if len(errs) > 0 {
		tt.Error = errs[0]
	}
	tt.EndTime = time.Now().UnixNano()
}

// ReleaseControl 返回追踪对象自身。该方法保留链式调用语义，不修改任何字段。
func (tt *TransitionTrace) ReleaseControl() *TransitionTrace {
	return tt
}

// SetToState 记录迁移规则声明的目标状态，并返回自身。
func (tt *TransitionTrace) SetToState(toState FSMState) *TransitionTrace {
	tt.ToState = toState

	return tt
}

// SetFinalState 记录事件处理结束后的实际状态，并返回自身。
func (tt *TransitionTrace) SetFinalState(finalState FSMState) *TransitionTrace {
	tt.FinalState = finalState

	return tt
}

// TransitionSuccess 将本次迁移标记为成功，并返回自身。
func (tt *TransitionTrace) TransitionSuccess() *TransitionTrace {
	tt.TransitionResult = true

	return tt
}

// TransitionFail 将本次迁移标记为失败，并返回自身。
func (tt *TransitionTrace) TransitionFail() *TransitionTrace {
	tt.TransitionResult = false

	return tt
}

// TrimPtr 返回追踪对象的值副本，避免将内部构建用指针暴露给调用方。
func (tt *TransitionTrace) TrimPtr() TransitionTrace {
	return *tt
}

// String 将迁移追踪格式化为便于日志记录和排查问题的字符串。
func (tt *TransitionTrace) String() string {
	return fmt.Sprintf("%v", util.PrettyMapStruct(tt, false))
}
