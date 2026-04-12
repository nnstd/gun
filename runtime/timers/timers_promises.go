package timers

import (
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func promiseResolve(value *jsvalue.JSValue) *jsvalue.JSValue {
	return promise.Promise.Get("resolve").Call(value)
}

var PromisesAsJSValue = jsvalue.ObjectFrom(
	"setTimeout", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		delay := 0
		value := jsvalue.NewUndefined()
		if len(args) > 0 && args[0] != nil {
			delay = int(args[0].Number())
		}
		if len(args) > 1 && args[1] != nil {
			value = args[1]
		}
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		return promiseResolve(value)
	}),
	"setImmediate", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		value := jsvalue.NewUndefined()
		if len(args) > 0 && args[0] != nil {
			value = args[0]
		}
		return promiseResolve(value)
	}),
	"setInterval", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		delay := 0
		value := jsvalue.NewUndefined()
		if len(args) > 0 && args[0] != nil {
			delay = int(args[0].Number())
		}
		if len(args) > 1 && args[1] != nil {
			value = args[1]
		}
		iter := jsvalue.NewObject()
		iter.Set("next", jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if delay > 0 {
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
			return promiseResolve(jsvalue.ObjectFrom(
				"value", value,
				"done", jsvalue.NewBool(false),
			))
		}))
		return iter
	}),
)
