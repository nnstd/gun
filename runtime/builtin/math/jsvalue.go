package math

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("floor", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Floor(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("ceil", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Ceil(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("round", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Round(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("abs", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Abs(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("max", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Max(args...)
	}))
	obj.Set("min", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Min(args...)
	}))
	obj.Set("sqrt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Sqrt(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("pow", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return Pow(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("random", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Random()
	}))
	obj.Set("log", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Log(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("log2", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Log2(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("trunc", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Trunc(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("sign", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return Sign(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("isNaN", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return IsNaN(args[0])
		}
		return jsvalue.NewBool(false)
	}))
	obj.Set("isFinite", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return IsFinite(args[0])
		}
		return jsvalue.NewBool(true)
	}))
	return obj
}()
