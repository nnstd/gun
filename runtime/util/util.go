package util

import (
	"fmt"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var AsJSValue = jsvalue.ObjectFrom(
	"format", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		format := args[0].String()
		vals := make([]any, 0, len(args)-1)
		for _, arg := range args[1:] {
			vals = append(vals, arg)
		}
		return jsvalue.NewString(fmt.Sprintf(format, vals...))
	}),
	"inspect", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("undefined")
		}
		return jsvalue.NewString(fmt.Sprint(args[0]))
	}),
	"inherits", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[0] != nil && args[1] != nil {
			args[0].SetPrototype(args[1])
		}
		return jsvalue.NewUndefined()
	}),
	"types", jsvalue.ObjectFrom(
		"isAnyArrayBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewBool(false)
			}
			return jsvalue.NewBool(strings.Contains(args[0].TypeString(), "object"))
		}),
	),
)
