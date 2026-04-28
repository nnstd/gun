package fs

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// AsJSValue returns the fs module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	ensureFSStreamClasses()
	obj := jsvalue.NewObject()
	obj.Set("ReadStream", ReadStream)
	obj.Set("WriteStream", WriteStream)
	obj.Set("readFileSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return ReadFileSync(args[0], args[1:]...)
		}
		if len(args) == 1 {
			return ReadFileSync(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("readFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewUndefined()
		}
		path := args[0]
		cbIndex := len(args) - 1
		cb := args[cbIndex]
		if cb == nil || cb.TypeString() != "function" {
			return jsvalue.NewUndefined()
		}
		opts := args[1:cbIndex]
		if errVal := abortErrFromSignal(optionSignal(opts...)); errVal != nil {
			cb.Call(errVal)
			return jsvalue.NewUndefined()
		}
		var out *jsvalue.JSValue
		var errVal *jsvalue.JSValue
		func() {
			defer func() {
				if r := recover(); r != nil {
					errVal = jsvalue.From(r)
				}
			}()
			out = ReadFile(path, opts...)
		}()
		if errVal != nil {
			cb.Call(errVal)
		} else {
			cb.Call(jsvalue.NewNull(), out)
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("writeFileSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return WriteFileSync(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("writeFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return callbackWrite(args, WriteFile)
	}))
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
	obj.Set("appendFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return callbackWrite(args, AppendFile)
	}))
	obj.Set("createReadStream", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return CreateReadStream(args[0], args[1:]...)
		}
		if len(args) == 1 {
			return CreateReadStream(args[0])
		}
		return CreateReadStream(jsvalue.NewString(""))
	}))
	obj.Set("createWriteStream", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return CreateWriteStream(args[0], args[1:]...)
		}
		if len(args) == 1 {
			return CreateWriteStream(args[0])
		}
		return CreateWriteStream(jsvalue.NewString(""))
	}))
	obj.Set("realpath", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return Realpath(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("promises", PromisesAsJSValue)
	return obj
}()

func callbackWrite(args []*jsvalue.JSValue, fn func(*jsvalue.JSValue, *jsvalue.JSValue, ...*jsvalue.JSValue) *jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		return jsvalue.NewUndefined()
	}
	path := args[0]
	data := args[1]
	cbIndex := len(args) - 1
	cb := args[cbIndex]
	if cb == nil || cb.TypeString() != "function" {
		return jsvalue.NewUndefined()
	}
	opts := args[2:cbIndex]
	var errVal *jsvalue.JSValue
	func() {
		defer func() {
			if r := recover(); r != nil {
				errVal = jsvalue.From(r)
			}
		}()
		fn(path, data, opts...)
	}()
	if errVal != nil {
		cb.Call(errVal)
	} else {
		cb.Call(jsvalue.NewNull())
	}
	return jsvalue.NewUndefined()
}
