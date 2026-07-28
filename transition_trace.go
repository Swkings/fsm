package fsm

import (
	"encoding/json"
	"fmt"
	"time"
)

// TransitionTrace 记录一次 ProcessEvent 调用从开始到结束的转移信息。
// 无论转移成功还是失败，调用方都可以通过它还原事件、状态和耗时。
type TransitionTrace struct {
	// TtId 是本次转移预生成的追踪标识。
	TtId string
	// PrevTtId 是转移开始前 FSM 持有的追踪标识，用于串联相邻转移。
	PrevTtId string
	// FromState 是处理事件前的状态。
	FromState FSMState
	// Event 是本次尝试处理的事件。
	Event FSMEvent
	// ToState 是转移规则声明的目标状态；规则不存在时保持零值。
	ToState FSMState
	// TransitionResult 表示目标状态是否成功提交。
	TransitionResult bool
	// FinalState 是本次处理结束时状态机实际所处的状态。
	FinalState FSMState
	// Error 保存转移失败的原因；成功时为 nil。
	Error error
	// EntryMetadata 是事件进入 FSM、任何 Handler 执行前的完整运行数据快照。
	EntryMetadata FSMMetadata
	// ExitMetadata 是事件完成或失败、最终退出 FSM 时的完整运行数据快照。
	ExitMetadata FSMMetadata
	// BeginTime 是转移开始时的 Unix 纳秒时间戳。
	BeginTime int64
	// EndTime 是转移结束时的 Unix 纳秒时间戳。
	EndTime int64

	beginTime time.Time
	endTime   time.Time
}

// NewTransitionTrace 创建转移追踪对象。调用 Start 后才会记录开始时间。
func NewTransitionTrace(prevTtId string, ttId string, fromState FSMState, event FSMEvent) *TransitionTrace {
	return &TransitionTrace{
		TtId:      ttId,
		PrevTtId:  prevTtId,
		FromState: fromState,
		Event:     event,
	}
}

// Start 记录当前 Unix 纳秒时间为转移开始时间，并返回自身以支持链式调用。
func (tt *TransitionTrace) Start() *TransitionTrace {
	tt.beginTime = time.Now()
	tt.BeginTime = tt.beginTime.UnixNano()

	return tt
}

// End 记录转移结束时间；传入多个错误时只保存第一个错误。
func (tt *TransitionTrace) End(errs ...error) {
	if len(errs) > 0 {
		tt.Error = errs[0]
	}
	tt.endTime = time.Now()
	tt.EndTime = tt.endTime.UnixNano()
}

// Duration 返回转移耗时。由 Start 和 End 创建的追踪使用 Go 单调时钟计算，
// 反序列化得到的追踪则回退为 Unix 纳秒时间戳之差。
func (tt *TransitionTrace) Duration() time.Duration {
	if !tt.beginTime.IsZero() && !tt.endTime.IsZero() {
		return tt.endTime.Sub(tt.beginTime)
	}
	if tt.EndTime <= tt.BeginTime {
		return 0
	}
	return time.Duration(tt.EndTime - tt.BeginTime)
}

// ReleaseControl 返回追踪对象自身。该方法保留链式调用语义，不修改任何字段。
func (tt *TransitionTrace) ReleaseControl() *TransitionTrace {
	return tt
}

// SetToState 记录转移规则声明的目标状态，并返回自身。
func (tt *TransitionTrace) SetToState(toState FSMState) *TransitionTrace {
	tt.ToState = toState

	return tt
}

// SetFinalState 记录事件处理结束后的实际状态，并返回自身。
func (tt *TransitionTrace) SetFinalState(finalState FSMState) *TransitionTrace {
	tt.FinalState = finalState

	return tt
}

// SetEntryMetadata 记录事件进入 FSM 时的完整运行数据快照。
func (tt *TransitionTrace) SetEntryMetadata(metadata FSMMetadata) *TransitionTrace {
	tt.EntryMetadata = metadata.clone()
	return tt
}

// SetExitMetadata 记录事件最终退出 FSM 时的完整运行数据快照。
func (tt *TransitionTrace) SetExitMetadata(metadata FSMMetadata) *TransitionTrace {
	tt.ExitMetadata = metadata.clone()
	return tt
}

// TransitionSuccess 将本次转移标记为成功，并返回自身。
func (tt *TransitionTrace) TransitionSuccess() *TransitionTrace {
	tt.TransitionResult = true

	return tt
}

// TransitionFail 将本次转移标记为失败，并返回自身。
func (tt *TransitionTrace) TransitionFail() *TransitionTrace {
	tt.TransitionResult = false

	return tt
}

// TrimPtr 返回追踪对象的值副本，避免将内部构建用指针暴露给调用方。
func (tt *TransitionTrace) TrimPtr() TransitionTrace {
	clone := *tt
	clone.EntryMetadata = tt.EntryMetadata.clone()
	clone.ExitMetadata = tt.ExitMetadata.clone()
	return clone
}

// String 将转移追踪格式化为便于日志记录和排查问题的字符串。
func (tt *TransitionTrace) String() string {
	errorMessage := ""
	if tt.Error != nil {
		errorMessage = tt.Error.Error()
	}
	view := struct {
		TtId             string      `json:"TtId"`
		PrevTtId         string      `json:"PrevTtId"`
		FromState        FSMState    `json:"FromState"`
		Event            FSMEvent    `json:"Event"`
		ToState          FSMState    `json:"ToState"`
		TransitionResult bool        `json:"TransitionResult"`
		FinalState       FSMState    `json:"FinalState"`
		Error            string      `json:"Error,omitempty"`
		EntryMetadata    FSMMetadata `json:"EntryMetadata"`
		ExitMetadata     FSMMetadata `json:"ExitMetadata"`
		BeginTime        int64       `json:"BeginTime"`
		EndTime          int64       `json:"EndTime"`
	}{
		TtId:             tt.TtId,
		PrevTtId:         tt.PrevTtId,
		FromState:        tt.FromState,
		Event:            tt.Event,
		ToState:          tt.ToState,
		TransitionResult: tt.TransitionResult,
		FinalState:       tt.FinalState,
		Error:            errorMessage,
		EntryMetadata:    tt.EntryMetadata.clone(),
		ExitMetadata:     tt.ExitMetadata.clone(),
		BeginTime:        tt.BeginTime,
		EndTime:          tt.EndTime,
	}

	data, err := json.Marshal(view)
	if err != nil {
		return fmt.Sprintf("%+v", *tt)
	}
	return string(data)
}
