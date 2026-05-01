package console

import (
	"fmt"
	"os"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/otel"
)

func toAny(args []*jsvalue.JSValue) []any {
	out := make([]any, len(args))
	for i, v := range args {
		out[i] = v
	}
	return out
}

func Log(args ...*jsvalue.JSValue) {
	fmt.Println(prefixSpan(toAny(args))...)
}

func Error(args ...*jsvalue.JSValue) {
	fmt.Fprintln(os.Stderr, prefixSpan(toAny(args))...)
}

func Warn(args ...*jsvalue.JSValue) {
	fmt.Fprintln(os.Stderr, prefixSpan(toAny(args))...)
}

func Dir(args ...*jsvalue.JSValue) {
	for _, a := range args {
		fmt.Printf("%+v\n", a)
	}
}

func prefixSpan(args []any) []any {
	if !otel.Enabled {
		return args
	}
	traceID, spanID := otel.SpanContext()
	if traceID == "" {
		return args
	}
	prefixed := make([]any, 0, len(args)+1)
	prefixed = append(prefixed, fmt.Sprintf("[%s/%s]", traceID, spanID))
	prefixed = append(prefixed, args...)
	return prefixed
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
