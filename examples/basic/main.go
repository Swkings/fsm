package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swkings/fsm"
)

type doorPayload struct {
	Name       string
	Authorized bool
	Stages     []string
}

func (p *doorPayload) JSON() ([]byte, error) { return json.Marshal(p) }
func (p *doorPayload) DisplayName() string   { return p.Name }
func (p *doorPayload) Category() string      { return "" }

func main() {
	payload := &doorPayload{Name: "仓库门", Authorized: true}
	machine := fsm.NewFSM("door", "closed", payload)
	defer func() { _ = machine.Close() }()

	machine.SetTransitions(fsm.NewTransitionItem(
		fsm.FSMEvent("open"),
		fsm.FSMState("closed"),
		fsm.FSMState("opened"),
		fsm.Handler[*doorPayload]{
			BeforeHandler: func(machine *fsm.FSM[*doorPayload], _ ...any) error {
				machine.Payload.Stages = append(machine.Payload.Stages, "before")
				if !machine.Payload.Authorized {
					return errors.New("没有开门权限")
				}

				return nil
			},
			Handler: func(machine *fsm.FSM[*doorPayload], _ ...any) error {
				machine.Payload.Stages = append(machine.Payload.Stages, "handler")
				fmt.Printf("正在打开%s\n", machine.Payload.Name)

				return nil
			},
			AfterHandler: func(machine *fsm.FSM[*doorPayload], _ ...any) error {
				machine.Payload.Stages = append(machine.Payload.Stages, "after")

				return nil
			},
		},
	))

	trace, err := machine.ProcessEvent("open")
	if err != nil {
		panic(err)
	}
	fmt.Printf("state=%s success=%t stages=%v\n", machine.GetCurrentState(), trace.TransitionResult, machine.Payload.Stages)
}
