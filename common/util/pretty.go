package util

import (
	"bytes"
	"encoding/json"
)

func PrettyMapStruct(v interface{}, indent bool) interface{} {
	jsonV, err := json.Marshal(v)
	if err != nil {
		return v
	}

	var out bytes.Buffer
	if indent {
		err = json.Indent(&out, jsonV, "", "  ")
	} else {
		out = *bytes.NewBuffer(jsonV)
	}

	if err != nil {
		return v
	}
	return out.String()
}
