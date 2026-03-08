package url

import jsvalue "github.com/nnstd/gun/runtime/builtin"

// AsJSValue returns the url module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("fileURLToPath", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 { return FileURLToPath(args[0]) }
		return jsvalue.NewString("")
	}))
	return obj
}()
