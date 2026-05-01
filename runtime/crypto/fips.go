package crypto

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func getFipsJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(0)
}

func setFipsJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	// No-op: FIPS not supported in pure Go
	return jsvalue.NewUndefined()
}

func secureHeapUsedJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("total", jsvalue.NewNumber(0))
	obj.Set("used", jsvalue.NewNumber(0))
	obj.Set("utilization", jsvalue.NewNumber(0))
	return obj
}

func setEngineJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	// No-op: custom engines not supported
	return jsvalue.NewUndefined()
}
