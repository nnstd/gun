//go:build !baseline

package jsonx

import "github.com/bytedance/sonic"

func Unmarshal(data []byte, v any) error {
	return sonic.ConfigStd.Unmarshal(data, v)
}
