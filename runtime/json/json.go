package json

import (
	stdjson "encoding/json"

	"github.com/nnstd/gun/runtime/jsvalue"
)

func Stringify(v *jsvalue.JSValue) *jsvalue.JSValue {
	b, _ := stdjson.Marshal(v.String())
	return jsvalue.NewString(string(b))
}

func Parse(s *jsvalue.JSValue) *jsvalue.JSValue {
	str := ""
	if s != nil {
		str = s.String()
	}
	var v any
	stdjson.Unmarshal([]byte(str), &v)
	return jsvalue.From(v)
}
