package stream

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

var PromisesAsJSValue = jsvalue.ObjectFrom(
	"pipeline", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Call(jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
			resolve := inner[0]
			reject := inner[1]
			cb := jsvalue.NewFunction(func(cbArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
				if len(cbArgs) > 0 && cbArgs[0] != nil && cbArgs[0].TypeString() != "undefined" && cbArgs[0].TypeString() != "null" {
					reject.Call(cbArgs[0])
				} else if len(args) > 0 {
					resolve.Call(args[len(args)-1])
				} else {
					resolve.Call(jsvalue.NewUndefined())
				}
				return jsvalue.NewUndefined()
			})
			streamArgs := append([]*jsvalue.JSValue{}, args...)
			streamArgs = append(streamArgs, cb)
			AsJSValue.Get("pipeline").Call(streamArgs...)
			return jsvalue.NewUndefined()
		}))
	}),
	"finished", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Call(jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
			resolve := inner[0]
			reject := inner[1]
			cb := jsvalue.NewFunction(func(cbArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
				if len(cbArgs) > 0 && cbArgs[0] != nil && cbArgs[0].TypeString() != "undefined" && cbArgs[0].TypeString() != "null" {
					reject.Call(cbArgs[0])
				} else if len(args) > 0 {
					resolve.Call(args[0])
				} else {
					resolve.Call(jsvalue.NewUndefined())
				}
				return jsvalue.NewUndefined()
			})
			streamArgs := append([]*jsvalue.JSValue{}, args...)
			streamArgs = append(streamArgs, cb)
			AsJSValue.Get("finished").Call(streamArgs...)
			return jsvalue.NewUndefined()
		}))
	}),
)
