package events

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

var EventEmitter *jsvalue.JSValue
var AsJSValue *jsvalue.JSValue

func init() {
	EventEmitter = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.Set("_events", jsvalue.NewObject())
		return nil
	}, nil)

	EventEmitter.Get("prototype").Set("on", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		event := args[1].String()
		events := args[0].Get("_events")
		list := events.Get(event)
		if !list.IsArray() {
			list = jsvalue.NewArray()
		}
		list.MethodCall("push", args[2])
		events.Set(event, list)
		return args[0]
	}).MarkAsMethod())

	EventEmitter.Get("prototype").Set("addListener", EventEmitter.Get("prototype").Get("on"))

	EventEmitter.Get("prototype").Set("once", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		var wrapper *jsvalue.JSValue
		wrapper = jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			args[0].MethodCall("removeListener", args[1], wrapper)
			return args[2].Call(callArgs...)
		})
		return args[0].MethodCall("on", args[1], wrapper)
	}).MarkAsMethod())

	EventEmitter.Get("prototype").Set("emit", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		event := args[1].String()
		handlers := args[0].Get("_events").Get(event)
		if !handlers.IsArray() {
			return jsvalue.NewBool(false)
		}
		for _, handler := range handlers.Array() {
			if handler != nil {
				handler.Call(args[2:]...)
			}
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	EventEmitter.Get("prototype").Set("removeListener", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		event := args[1].String()
		handlers := args[0].Get("_events").Get(event)
		if !handlers.IsArray() {
			return args[0]
		}
		next := jsvalue.NewArray()
		for _, handler := range handlers.Array() {
			if handler != args[2] {
				next.MethodCall("push", handler)
			}
		}
		args[0].Get("_events").Set(event, next)
		return args[0]
	}).MarkAsMethod())

	EventEmitter.Get("prototype").Set("off", EventEmitter.Get("prototype").Get("removeListener"))

	EventEmitter.Get("prototype").Set("removeAllListeners", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		if len(args) > 1 && args[1] != nil {
			args[0].Get("_events").Set(args[1].String(), jsvalue.NewArray())
		} else {
			args[0].Set("_events", jsvalue.NewObject())
		}
		return args[0]
	}).MarkAsMethod())

	EventEmitter.Get("prototype").Set("listeners", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewArray()
		}
		list := args[0].Get("_events").Get(args[1].String())
		if list.IsArray() {
			return list
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())

	AsJSValue = jsvalue.ObjectFrom(
		"EventEmitter", EventEmitter,
		"once", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) < 2 || args[0] == nil {
				return promise.Promise.Get("resolve").Call(jsvalue.NewArray())
			}
			emitter := args[0]
			event := args[1]
			return promise.Promise.Call(jsvalue.NewFunction(func(inner ...*jsvalue.JSValue) *jsvalue.JSValue {
				resolve := inner[0]
				reject := inner[1]
				handler := jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
					resolve.Call(jsvalue.NewArray(callArgs...))
					return jsvalue.NewUndefined()
				})
				emitter.MethodCall("once", event, handler)
				if event.String() != "error" {
					emitter.MethodCall("once", jsvalue.NewString("error"), jsvalue.NewFunction(func(callArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
						if len(callArgs) > 0 {
							reject.Call(callArgs[0])
						} else {
							reject.Call(jsvalue.NewUndefined())
						}
						return jsvalue.NewUndefined()
					}))
				}
				return jsvalue.NewUndefined()
			}))
		}),
	)
}
