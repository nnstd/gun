//go:build baseline

package jsonx

import "encoding/json"

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
