package console

import (
	"fmt"
	"os"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func toAny(args []*jsvalue.JSValue) []any {
	out := make([]any, len(args))
	for i, v := range args {
		out[i] = v
	}
	return out
}

func Log(args ...*jsvalue.JSValue) {
	fmt.Println(toAny(args)...)
}

func Error(args ...*jsvalue.JSValue) {
	fmt.Fprintln(os.Stderr, toAny(args)...)
}

func Warn(args ...*jsvalue.JSValue) {
	fmt.Fprintln(os.Stderr, toAny(args)...)
}

func Dir(args ...*jsvalue.JSValue) {
	for _, a := range args {
		fmt.Printf("%+v\n", a)
	}
}

// AsJSValue returns console as a callable JSValue object.
var AsJSValue = jsvalue.NewObject()

func init() {
	AsJSValue.Set("log", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		Log(args...)
		return jsvalue.NewUndefined()
	}))
	AsJSValue.Set("error", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		Error(args...)
		return jsvalue.NewUndefined()
	}))
	AsJSValue.Set("warn", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		Warn(args...)
		return jsvalue.NewUndefined()
	}))
	AsJSValue.Set("dir", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		Dir(args...)
		return jsvalue.NewUndefined()
	}))
	jsvalue.RegisterGlobal("console", AsJSValue)
}
