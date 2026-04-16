package eventloop

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func init() {
	jsvalue.RegisterGlobal("setTimeout", jsvalue.NewFunction(SetTimeout))
	jsvalue.RegisterGlobal("setInterval", jsvalue.NewFunction(SetInterval))
	jsvalue.RegisterGlobal("setImmediate", jsvalue.NewFunction(SetImmediate))
	jsvalue.RegisterGlobal("clearTimeout", jsvalue.NewFunction(ClearTimeout))
	jsvalue.RegisterGlobal("clearInterval", jsvalue.NewFunction(ClearInterval))
	jsvalue.RegisterGlobal("clearImmediate", jsvalue.NewFunction(ClearImmediate))
}
