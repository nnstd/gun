package zlib

import (
	"bytes"
	gziplib "compress/gzip"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func gzipValue(v *jsvalue.JSValue) *jsvalue.JSValue {
	if v == nil {
		return jsvalue.NewString("")
	}
	var buf bytes.Buffer
	zw := gziplib.NewWriter(&buf)
	_, _ = zw.Write([]byte(v.String()))
	zw.Close()
	return jsvalue.NewString(buf.String())
}

var AsJSValue = jsvalue.ObjectFrom(
	"gzip", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return gzipValue(args[0])
	}),
)

var PromisesAsJSValue = jsvalue.ObjectFrom(
	"gzip", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return promise.Promise.Get("resolve").Call(jsvalue.NewString(""))
		}
		return promise.Promise.Get("resolve").Call(gzipValue(args[0]))
	}),
)
