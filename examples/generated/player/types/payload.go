package types

import "encoding/json"

// AttackParams 是 HIT_LIGHT、HIT_HEAVY 事件携带的攻击参数。
type AttackParams struct {
	Damage int
}

// Payload 保存示例角色在状态转移过程中变化的少量业务数据。
type Payload struct {
	Name             string `json:"name"`
	Health           int    `json:"health"`
	TargetHealth     int    `json:"targetHealth"`
	Stamina          int    `json:"stamina"`
	MoveSpeed        int    `json:"moveSpeed"`
	VerticalSpeed    int    `json:"verticalSpeed"`
	IncomingDamage   int    `json:"incomingDamage,omitempty"`
	LastAttackDamage int    `json:"lastAttackDamage,omitempty"`
	LastEvent        string `json:"lastEvent,omitempty"`
	ActionCount      int    `json:"actionCount"`
}

func (p *Payload) JSON() ([]byte, error) {
	return json.Marshal(p)
}

func (p *Payload) DisplayName() string {
	return p.Name
}

func (p *Payload) Category() string {
	return "player"
}
