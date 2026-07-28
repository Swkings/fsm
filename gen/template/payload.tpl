package types

import "encoding/json"

type Payload struct {
	// TODO: your payload
}

func (p Payload) JSON() ([]byte, error) {
	return json.Marshal(p)
}

func (p Payload) DisplayName() string {
	// TODO: your DisplayName
	return "DisplayName"
}

func (p Payload) Path() string {
	// TODO: your Path
	return "fsm/path"
}