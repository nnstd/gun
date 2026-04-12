package fs

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// AsJSValue returns the fs module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("readFileSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return ReadFileSync(args[0], args[1:]...)
		}
		if len(args) == 1 {
			return ReadFileSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("readFile", obj.Get("readFileSync"))
	obj.Set("writeFileSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return WriteFileSync(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("writeFile", obj.Get("writeFileSync"))
	obj.Set("existsSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return ExistsSync(args[0])
		}
		return jsvalue.NewBool(false)
	}))
	obj.Set("mkdirSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return MkdirSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("readdirSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return ReaddirSync(args[0])
		}
		return jsvalue.NewArray()
	}))
	obj.Set("unlinkSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return UnlinkSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("statSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return StatSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("rmdirSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return RmdirSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("appendFileSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return AppendFileSync(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("realpath", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return Realpath(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("promises", obj)
	return obj
}()
