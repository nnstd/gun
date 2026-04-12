package json

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("stringify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Stringify(args[0])
		}
		return jsvalue.NewString("")
	}))
	obj.Set("parse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Parse(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	return obj
}()
