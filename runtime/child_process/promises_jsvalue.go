package child_process

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

var PromisesAsJSValue = jsvalue.ObjectFrom(
	"exec", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewUndefined())
		}
		return promise.Promise.Get("resolve").Call(runCommand(args[0].String()))
	}),
	"execFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return promise.Promise.Get("resolve").Call(jsvalue.NewUndefined())
		}
		return promise.Promise.Get("resolve").Call(runCommand(args[0].String()))
	}),
)
