package path

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// AsJSValue returns the path module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("join", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Join(args...)
	}))
	obj.Set("resolve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Resolve(args...)
	}))
	obj.Set("basename", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return Basename(args[0]) }
		return jsvalue.NewString("")
	}))
	obj.Set("dirname", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return Dirname(args[0]) }
		return jsvalue.NewString("")
	}))
	obj.Set("extname", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return Extname(args[0]) }
		return jsvalue.NewString("")
	}))
	obj.Set("relative", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 { return Relative(args[0], args[1]) }
		return jsvalue.NewString("")
	}))
	obj.Set("isAbsolute", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return IsAbsolute(args[0]) }
		return jsvalue.NewBool(false)
	}))
	obj.Set("normalize", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return Normalize(args[0]) }
		return jsvalue.NewString("")
	}))
	obj.Set("sep", Sep)
	obj.Set("delimiter", Delimiter)
	return obj
}()
