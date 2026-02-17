package json

import (
	stdjson "encoding/json"

	"github.com/nnstd/gun/runtime/jsvalue"
)

func Stringify(v any) string {
	b, _ := stdjson.Marshal(v)
	return string(b)
}

func Parse(s string) *jsvalue.JSValue {
	var v any
	stdjson.Unmarshal([]byte(s), &v)
	return jsvalue.From(v)
}
