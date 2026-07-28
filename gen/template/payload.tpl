package types

import "encoding/json"

type Payload struct {
	// TODO: your payload
}

func (p Payload) JSON() ([]byte, error) {
	return json.Marshal(p)
}
