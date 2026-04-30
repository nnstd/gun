package os

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/constants"
)

// AsJSValue returns the os module as a JSValue object with all exports.
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("homedir", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Homedir() }))
	obj.Set("tmpdir", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Tmpdir() }))
	obj.Set("hostname", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Hostname() }))
	obj.Set("platform", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Platform() }))
	obj.Set("arch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Arch() }))
	obj.Set("cpus", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Cpus() }))
	obj.Set("environ", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Environ() }))
	obj.Set("constants", constants.OSConstants)
	obj.Set("EOL", EOL)
	return obj
}()
