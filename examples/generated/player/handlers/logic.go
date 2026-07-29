package handlers

import (
	"errors"
	"fmt"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player/types"
)

var stateDescriptions = map[fsm.FSMState]string{
	types.StateEnum.IDLE:         "站立闲置",
	types.StateEnum.WALK:         "行走",
	types.StateEnum.RUN:          "奔跑",
	types.StateEnum.JUMP_START:   "起跳准备",
	types.StateEnum.IN_AIR:       "空中滞留",
	types.StateEnum.ATTACK_LIGHT: "轻攻击",
	types.StateEnum.ATTACK_HEAVY: "重攻击",
	types.StateEnum.HURT:         "受击硬直",
	types.StateEnum.DIE:          "死亡",
}

var eventDescriptions = map[fsm.FSMEvent]string{
	types.EventEnum.MOVE:      "输入移动",
	types.EventEnum.RUN:       "输入奔跑",
	types.EventEnum.STOP:      "停止输入",
	types.EventEnum.JUMP:      "按下跳跃",
	types.EventEnum.HIT_LIGHT: "按下轻击",
	types.EventEnum.HIT_HEAVY: "按下重击",
	types.EventEnum.ANIM_END:  "动画结束",
	types.EventEnum.LAND:      "着地",
	types.EventEnum.DAMAGE:    "受到伤害",
	types.EventEnum.DEAD:      "生命值归零",
	types.EventEnum.REVIVE:    "复活",
}

// handleTransition 是示例业务逻辑入口。每个由生成器创建的 Handler 文件
// 仍然独立绑定一条转移，只把重复的数据更新委托给这个辅助函数。
func handleTransition(machine *fsm.FSM[*types.Payload], event fsm.FSMEvent) error {
	payload := machine.Payload
	fromState := machine.GetCurrentState()
	payload.LastEvent = event.String()
	payload.ActionCount++

	switch event {
	case types.EventEnum.MOVE:
		payload.MoveSpeed = 3
	case types.EventEnum.RUN:
		payload.MoveSpeed = 7
		payload.Stamina = max(0, payload.Stamina-2)
	case types.EventEnum.STOP:
		payload.MoveSpeed = 0
	case types.EventEnum.JUMP:
		payload.VerticalSpeed = 12
		payload.Stamina = max(0, payload.Stamina-8)
	case types.EventEnum.HIT_LIGHT:
		payload.MoveSpeed = 0
		payload.Stamina = max(0, payload.Stamina-10)
	case types.EventEnum.HIT_HEAVY:
		payload.MoveSpeed = 0
		payload.Stamina = max(0, payload.Stamina-20)
	case types.EventEnum.ANIM_END:
		if fromState == types.StateEnum.JUMP_START {
			payload.VerticalSpeed = 8
		}
		payload.IncomingDamage = 0
	case types.EventEnum.LAND:
		payload.VerticalSpeed = 0
		payload.MoveSpeed = 0
	case types.EventEnum.DAMAGE:
		damage := payload.IncomingDamage
		if damage <= 0 {
			damage = 20
		}
		payload.Health = max(0, payload.Health-damage)
		payload.MoveSpeed = 0
		payload.VerticalSpeed = 0
	case types.EventEnum.DEAD:
		payload.Health = 0
		payload.MoveSpeed = 0
		payload.VerticalSpeed = 0
	case types.EventEnum.REVIVE:
		payload.Health = 100
		payload.Stamina = 100
		payload.MoveSpeed = 0
		payload.VerticalSpeed = 0
		payload.IncomingDamage = 0
	}

	payload.History = append(payload.History, fmt.Sprintf(
		"%s(%s) -- %s(%s)",
		fromState,
		stateDescriptions[fromState],
		event,
		eventDescriptions[event],
	))
	return nil
}

// handleAttackTransition 校验攻击事件参数，完成状态业务更新后扣除目标指定血量。
func handleAttackTransition(
	machine *fsm.FSM[*types.Payload],
	event fsm.FSMEvent,
	params ...any,
) error {
	attack, err := parseAttackParams(params)
	if err != nil {
		return err
	}
	if err := handleTransition(machine, event); err != nil {
		return err
	}

	machine.Payload.TargetHealth = max(0, machine.Payload.TargetHealth-attack.Damage)
	machine.Payload.LastAttackDamage = attack.Damage
	return nil
}

func parseAttackParams(params []any) (types.AttackParams, error) {
	if len(params) == 0 {
		return types.AttackParams{}, errors.New("attack event requires types.AttackParams")
	}

	var attack types.AttackParams
	switch value := params[0].(type) {
	case types.AttackParams:
		attack = value
	case *types.AttackParams:
		if value == nil {
			return types.AttackParams{}, errors.New("attack params is nil")
		}
		attack = *value
	default:
		return types.AttackParams{}, fmt.Errorf("invalid attack params type %T", params[0])
	}
	if attack.Damage <= 0 {
		return types.AttackParams{}, errors.New("attack damage must be greater than zero")
	}
	return attack, nil
}
