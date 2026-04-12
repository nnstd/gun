package assert

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// AsJSValue returns the assert module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("strictEqual", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 { return StrictEqual(args[0], args[1]) }
		return jsvalue.NewUndefined()
	}))
	obj.Set("notStrictEqual", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 { return NotStrictEqual(args[0], args[1]) }
		return jsvalue.NewUndefined()
	}))
	obj.Set("strict", obj)
	return obj
}()
