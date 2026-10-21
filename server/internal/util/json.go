package util

import "encoding/json"

var jsonMarshal = json.Marshal

// Unmarshal decodes b into v, ignoring empty input.
func Unmarshal(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}
