package timers

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

var AsJSValue = jsvalue.ObjectFrom(
	"setTimeout", jsvalue.NewFunction(eventloop.SetTimeout),
	"setInterval", jsvalue.NewFunction(eventloop.SetInterval),
	"setImmediate", jsvalue.NewFunction(eventloop.SetImmediate),
	"clearTimeout", jsvalue.NewFunction(eventloop.ClearTimeout),
	"clearInterval", jsvalue.NewFunction(eventloop.ClearInterval),
	"clearImmediate", jsvalue.NewFunction(eventloop.ClearImmediate),
)
