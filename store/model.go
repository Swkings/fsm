// Package store 提供 FSM 数据持久化能力，与可视化实现无关。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrNotFound = errors.New("fsm store record not found")

type Metadata struct {
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

type Transition struct {
	FromState string `json:"fromState"`
	Event     string `json:"event"`
	ToState   string `json:"toState"`
}

type Trace struct {
	TransitionID string   `json:"transitionId"`
	PreviousID   string   `json:"previousId,omitempty"`
	FromState    string   `json:"fromState"`
	Event        string   `json:"event"`
	ToState      string   `json:"toState"`
	FinalState   string   `json:"finalState"`
	Success      bool     `json:"success"`
	Error        string   `json:"error,omitempty"`
	Entry        Metadata `json:"entry"`
	Exit         Metadata `json:"exit"`
	BeginTime    int64    `json:"beginTime"`
	EndTime      int64    `json:"endTime"`
}

// Record 是一次 FSM 事件处理完成后的持久化记录。Key 由调用方指定，
// 用于在进程重启后定位同一个业务状态机实例。
type Record struct {
	ID          string       `json:"id"`
	Key         string       `json:"key"`
	DisplayName string       `json:"displayName,omitempty"`
	Category    string       `json:"category,omitempty"`
	RecordedAt  time.Time    `json:"recordedAt"`
	Success     bool         `json:"success"`
	Metadata    Metadata     `json:"metadata"`
	Transitions []Transition `json:"transitions,omitempty"`
	Trace       Trace        `json:"trace"`
}

type Query struct {
	Key            string
	Limit          int
	SuccessfulOnly bool
}

type Descriptor struct {
	Driver   string `json:"driver"`
	Location string `json:"location"`
}

// Store 是 FSM 持久化后端的统一接口。
type Store interface {
	Save(context.Context, Record) error
	List(context.Context, Query) ([]Record, error)
	Latest(context.Context, string) (Record, error)
	Get(context.Context, string) (Record, error)
	Delete(context.Context, string) error
	Descriptor() Descriptor
	Close() error
}
