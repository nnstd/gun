package dns

import (
	"net"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func resolve(v *jsvalue.JSValue) *jsvalue.JSValue {
	return promise.Promise.Get("resolve").Call(v)
}

func lookupValue(host *jsvalue.JSValue) *jsvalue.JSValue {
	if host == nil {
		return jsvalue.NewObject()
	}
	ip, err := net.ResolveIPAddr("ip", host.String())
	if err != nil || ip == nil {
		return jsvalue.NewObject()
	}
	family := 4
	if ip.IP.To4() == nil {
		family = 6
	}
	return jsvalue.ObjectFrom(
		"address", jsvalue.NewString(ip.IP.String()),
		"family", jsvalue.NewNumber(float64(family)),
	)
}

var AsJSValue = jsvalue.ObjectFrom(
	"lookup", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return lookupValue(jsvalue.NewString(""))
		}
		return lookupValue(args[0])
	}),
)

var PromisesAsJSValue = jsvalue.ObjectFrom(
	"lookup", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return resolve(jsvalue.NewObject())
		}
		return resolve(lookupValue(args[0]))
	}),
	"resolve4", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return resolve(jsvalue.NewArray())
		}
		ips, err := net.LookupIP(args[0].String())
		if err != nil {
			return resolve(jsvalue.NewArray())
		}
		var out []*jsvalue.JSValue
		for _, ip := range ips {
			if v4 := ip.To4(); v4 != nil {
				out = append(out, jsvalue.NewString(v4.String()))
			}
		}
		return resolve(jsvalue.NewArray(out...))
	}),
)
