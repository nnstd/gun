//go:build !baseline

package jsonx

import "github.com/bytedance/sonic"

func Unmarshal(data []byte, v any) error {
	return sonic.ConfigStd.Unmarshal(data, v)
}

func UnmarshalString(data string, v any) error {
	return sonic.ConfigStd.UnmarshalFromString(data, v)
}
