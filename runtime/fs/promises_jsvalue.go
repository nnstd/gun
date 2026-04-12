package fs

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

var PromisesAsJSValue = func() *jsvalue.JSValue {
	resolve := promise.Promise.Get("resolve")
	obj := jsvalue.NewObject()
	obj.Set("readFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 1 {
			out = ReadFile(args[0])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("writeFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 2 {
			out = WriteFile(args[0], args[1])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
	}))
	obj.Set("appendFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var out *jsvalue.JSValue
		if len(args) >= 2 {
			out = AppendFile(args[0], args[1])
		} else {
			out = jsvalue.NewUndefined()
		}
		return resolve.Call(out)
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
