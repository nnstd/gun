package fs

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

var PromisesAsJSValue = func() *jsvalue.JSValue {
	resolve := promise.Promise.Get("resolve")
	obj := jsvalue.NewObject()
	obj.Set("readFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 {
			return resolve.Call(jsvalue.NewUndefined())
		}
		return promiseResult(func() *jsvalue.JSValue { return ReadFile(args[0], args[1:]...) })
	}))
	obj.Set("writeFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return resolve.Call(jsvalue.NewUndefined())
		}
		return promiseResult(func() *jsvalue.JSValue { return WriteFile(args[0], args[1], args[2:]...) })
	}))
	obj.Set("appendFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return resolve.Call(jsvalue.NewUndefined())
		}
		return promiseResult(func() *jsvalue.JSValue { return AppendFile(args[0], args[1], args[2:]...) })
	}))
	obj.Set("copyFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 2 {
			out = CopyFile(args[0], args[1])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("rename", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 2 {
			out = Rename(args[0], args[1])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("mkdir", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Mkdir(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("readdir", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Readdir(args[0])
		} else {
			out = jsvalue.NewArray()
		}
		return resolve.Call(out)
	}))
	obj.Set("unlink", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Unlink(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("stat", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Stat(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("lstat", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Lstat(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("rm", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Rm(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("access", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Access(args[0])
		} else {
			out = jsvalue.NewBool(false)
		}
		return resolve.Call(out)
	}))
	obj.Set("realpath", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = Realpath(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	return obj
}()

func promiseResult(fn func() *jsvalue.JSValue) *jsvalue.JSValue {
	var out *jsvalue.JSValue
	var errVal *jsvalue.JSValue
	func() {
		defer func() {
			if r := recover(); r != nil {
				errVal = jsvalue.From(r)
			}
		}()
		out = fn()
	}()
	if errVal != nil {
		return promise.Promise.Get("reject").Call(errVal)
	}
	return promise.Promise.Get("resolve").Call(out)
}
