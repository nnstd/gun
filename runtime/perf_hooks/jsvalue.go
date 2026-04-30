package perf_hooks

import (
	"fmt"

	jserror "github.com/nnstd/gun/runtime/builtin/error"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var validEntryTypes = map[string]bool{
	"mark":     true,
	"measure":  true,
	"gc":       true,
	"function": true,
	"resource": true,
	"http":     true,
	"http2":    true,
	"timerify": true,
	"net":      true,
	"dns":      true,
}

// arg returns the i-th argument, or nil if not provided.
func arg(args []*jsvalue.JSValue, i int) *jsvalue.JSValue {
	if i >= len(args) {
		return nil
	}
	return args[i]
}

// Timerify wraps a function to measure its execution time.
func Timerify(fn, options *jsvalue.JSValue) *jsvalue.JSValue {
	if fn == nil || fn.Type() != jsvalue.TypeFunction {
		return jsvalue.NewUndefined()
	}
	return jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		start := PerformanceNow().Number()
		result := fn.Call(args...)
		elapsed := PerformanceNow().Number() - start
		_ = NewFunctionEntry("", nil, elapsed)
		return result
	})
}

func registerObserver(obs *jsvalue.JSValue) {
	observers = append(observers, observer{
		callback:   obs.Get("__callback"),
		entryTypes: extractEntryTypes(obs.Get("__entryTypes")),
		buffered:   nil,
	})
}

func unregisterObserver(obs *jsvalue.JSValue) {
	cb := obs.Get("__callback")
	for i, o := range observers {
		if o.callback == cb {
			observers = append(observers[:i], observers[i+1:]...)
			return
		}
	}
}

func deliverBuffered(obs *jsvalue.JSValue) {
	entryTypes := extractEntryTypes(obs.Get("__entryTypes"))
	var matching []perfEntry
	for _, e := range entries {
		for _, t := range entryTypes {
			if e.entryType == t {
				matching = append(matching, e)
			}
		}
	}
	if len(matching) > 0 {
		cb := obs.Get("__callback")
		if cb != nil && cb.Type() == jsvalue.TypeFunction {
			list := entriesToJSValue(matching)
			cb.Call(list)
		}
	}
}

func extractEntryTypes(v *jsvalue.JSValue) []string {
	if v == nil || !v.IsArray() {
		return nil
	}
	arr := v.Array()
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		result = append(result, item.String())
	}
	return result
}

var (
	// Performance is the perf_hooks.performance singleton object.
	Performance *jsvalue.JSValue
	// AsJSValue is the module export object for perf_hooks.
	AsJSValue *jsvalue.JSValue
)

func init() {
	perf := jsvalue.NewObject()

	perf.Set("now", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return PerformanceNow()
	}))

	perf.Set("mark", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Mark(arg(args, 0), arg(args, 1))
	}))

	perf.Set("measure", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Measure(arg(args, 0), arg(args, 1), arg(args, 2))
	}))

	perf.Set("clearMarks", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ClearMarks(arg(args, 0))
		return jsvalue.NewUndefined()
	}))

	perf.Set("clearMeasures", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ClearMeasures(arg(args, 0))
		return jsvalue.NewUndefined()
	}))

	perf.Set("clearResourceTimings", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ClearResourceTimings(arg(args, 0))
		return jsvalue.NewUndefined()
	}))

	perf.Set("getEntries", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return GetEntries()
	}))

	perf.Set("getEntriesByName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return GetEntriesByName(arg(args, 0), arg(args, 1))
	}))

	perf.Set("getEntriesByType", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return GetEntriesByType(arg(args, 0))
	}))

	perf.Set("setResourceTimingBufferSize", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		SetResourceTimingBufferSize(arg(args, 0))
		return jsvalue.NewUndefined()
	}))

	perf.Set("toJSON", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return ToJSON()
	}))

	perf.Set("markResourceTiming", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 7 {
			return jsvalue.NewUndefined()
		}
		return MarkResourceTiming(args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7:]...)
	}))

	perf.Set("timerify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Timerify(arg(args, 0), arg(args, 1))
	}))

	perf.Set("eventLoopUtilization", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return EventLoopUtilization(args...)
	}))

	perf.Set("timeOrigin", TimeOrigin())
	perf.Set("nodeTiming", GetNodeTiming())

	Performance = perf

	// Module exports
	exports := jsvalue.NewObject()
	exports.Set("performance", Performance)

	exports.Set("PerformanceObserver", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		cb := arg(args, 0)
		if cb == nil || cb.Type() != jsvalue.TypeFunction {
			panic(jserror.TypeError.Call(jsvalue.NewString(fmt.Sprintf("The \"callback\" argument must be of type Function. Received %s", cb.TypeString()))))
		}

		obs := jsvalue.NewObject()
		obs.Set("__callback", cb)
		obs.Set("__entryTypes", jsvalue.NewArray())
		obs.Set("__buffered", jsvalue.NewArray())

		obs.Set("observe", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			opts := arg(innerArgs, 0)
			if opts == nil || opts.Type() != jsvalue.TypeObject {
				panic(jserror.TypeError.Call(jsvalue.NewString("The \"options\" argument must be of type object. Received undefined")))
			}

			types := opts.Get("entryTypes")
			singleType := opts.Get("type")

			if types != nil && types.IsArray() {
				arr := types.Array()
				if len(arr) == 0 {
					panic(jserror.TypeError.Call(jsvalue.NewString("entryTypes must not be empty")))
				}
				obs.Set("__entryTypes", types)
			} else if singleType != nil && singleType.Type() != jsvalue.TypeUndefined {
				if !validEntryTypes[singleType.String()] {
					panic(jserror.TypeError.Call(jsvalue.NewString(fmt.Sprintf("'%s' is not a valid entry type", singleType.String()))))
				}
				obs.Set("__entryTypes", jsvalue.NewArray(singleType))
			} else {
				panic(jserror.TypeError.Call(jsvalue.NewString("Required entryTypes or type attribute")))
			}

			registerObserver(obs)
			if buf := opts.Get("buffered"); buf != nil && buf.Bool() {
				deliverBuffered(obs)
			}
			return jsvalue.NewUndefined()
		}))

		obs.Set("disconnect", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			unregisterObserver(obs)
			return jsvalue.NewUndefined()
		}))

		obs.Set("takeRecords", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			buf := obs.Get("__buffered")
			obs.Set("__buffered", jsvalue.NewArray())
			return buf
		}))

		return obs
	}))

	exports.Set("PerformanceObserverEntryList", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		list := jsvalue.NewObject()
		entriesArr := arg(args, 0)
		if entriesArr == nil {
			entriesArr = jsvalue.NewArray()
		}
		list.Set("__entries", entriesArr)

		list.Set("getEntries", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			return list.Get("__entries")
		}))

		list.Set("getEntriesByName", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			nameStr := ""
			if arg(innerArgs, 0) != nil {
				nameStr = arg(innerArgs, 0).String()
			}
			typeStr := ""
			if arg(innerArgs, 1) != nil {
				typeStr = arg(innerArgs, 1).String()
			}
			all := list.Get("__entries")
			if all == nil || !all.IsArray() {
				return jsvalue.NewArray()
			}
			arr := all.Array()
			filtered := make([]*jsvalue.JSValue, 0)
			for _, e := range arr {
				if e == nil {
					continue
				}
				n := e.Get("name")
				t := e.Get("entryType")
				if (nameStr == "" || (n != nil && n.String() == nameStr)) &&
					(typeStr == "" || (t != nil && t.String() == typeStr)) {
					filtered = append(filtered, e)
				}
			}
			return jsvalue.NewArray(filtered...)
		}))

		list.Set("getEntriesByType", jsvalue.NewFunction(func(innerArgs ...*jsvalue.JSValue) *jsvalue.JSValue {
			typeStr := ""
			if arg(innerArgs, 0) != nil {
				typeStr = arg(innerArgs, 0).String()
			}
			all := list.Get("__entries")
			if all == nil || !all.IsArray() {
				return jsvalue.NewArray()
			}
			arr := all.Array()
			filtered := make([]*jsvalue.JSValue, 0)
			for _, e := range arr {
				if e == nil {
					continue
				}
				t := e.Get("entryType")
				if t != nil && t.String() == typeStr {
					filtered = append(filtered, e)
				}
			}
			return jsvalue.NewArray(filtered...)
		}))

		return list
	}))

	exports.Set("constants", GetConstants())
	exports.Set("createHistogram", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return CreateHistogram(args...)
	}))
	exports.Set("eventLoopUtilization", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return EventLoopUtilization(args...)
	}))
	exports.Set("monitorEventLoopDelay", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return MonitorEventLoopDelay(args...)
	}))
	exports.Set("timerify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Timerify(arg(args, 0), arg(args, 1))
	}))

	AsJSValue = exports
}
