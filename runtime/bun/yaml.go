package bun

import (
	"strings"

	"github.com/goccy/go-yaml"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	error "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/module"
)

func YAMLAsJSValue() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("parse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		text := ""
		if len(args) > 0 && args[0] != nil {
			text = args[0].String()
		}
		var value any
		if err := yaml.Unmarshal([]byte(text), &value); err != nil {
			panic(error.SyntaxError.Call(jsvalue.NewString(err.Error())))
		}
		return module.DataToJSValue(module.NormalizeYAMLValue(value))
	}))
	obj.Set("stringify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var input *jsvalue.JSValue
		if len(args) > 0 {
			input = args[0]
		}
		opts := []yaml.EncodeOption{}
		if len(args) < 3 || args[2] == nil || args[2].TypeString() == "undefined" || args[2].TypeString() == "null" {
			opts = append(opts, yaml.Flow(true))
		} else {
			opts = append(opts, yaml.Indent(yamlSpace(args[2])))
		}
		out, err := yaml.MarshalWithOptions(jsValueToNative(input), opts...)
		if err != nil {
			panic(error.Error.Call(jsvalue.NewString(err.Error())))
		}
		return jsvalue.NewString(strings.TrimRight(string(out), "\n"))
	}))
	return obj
}
