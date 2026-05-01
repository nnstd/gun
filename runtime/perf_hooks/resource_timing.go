package perf_hooks

import jsvalue "github.com/nnstd/gun/runtime/builtin"

type perfEntry struct {
	name      string
	entryType string
	startTime float64
	duration  float64
	detail    *jsvalue.JSValue
}

func safeFloat(v *jsvalue.JSValue, key string) float64 {
	if v == nil {
		return 0
	}
	p := v.Get(key)
	if p == nil {
		return 0
	}
	return p.Number()
}

func MarkResourceTiming(timingInfo, requestedUrl, initiatorType, global, cacheMode, bodyInfo, responseStatus *jsvalue.JSValue, deliveryType ...*jsvalue.JSValue) *jsvalue.JSValue {
	name := requestedUrl.String()
	startTime := safeFloat(timingInfo, "startTime")
	if startTime == 0 {
		startTime = Now()
	}
	duration := safeFloat(timingInfo, "duration")

	e := perfEntry{
		name:      name,
		entryType: "resource",
		startTime: startTime,
		duration:  duration,
	}

	entry := createResourceTimingJSValue(e, timingInfo)

	detail := jsvalue.NewObject()
	detail.Set("initiatorType", initiatorType)
	detail.Set("cacheMode", cacheMode)
	detail.Set("bodyInfo", bodyInfo)
	detail.Set("responseStatus", responseStatus)
	if len(deliveryType) > 0 {
		detail.Set("deliveryType", deliveryType[0])
	}
	entry.Set("detail", detail)

	return entry
}

func createResourceTimingJSValue(e perfEntry, timingInfo *jsvalue.JSValue) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString(e.name))
	obj.Set("entryType", jsvalue.NewString(e.entryType))
	obj.Set("startTime", jsvalue.NewNumber(e.startTime))
	obj.Set("duration", jsvalue.NewNumber(e.duration))

	obj.Set("workerStart", jsvalue.NewNumber(safeFloat(timingInfo, "workerStart")))
	obj.Set("redirectStart", jsvalue.NewNumber(safeFloat(timingInfo, "redirectStart")))
	obj.Set("redirectEnd", jsvalue.NewNumber(safeFloat(timingInfo, "redirectEnd")))
	obj.Set("fetchStart", jsvalue.NewNumber(safeFloat(timingInfo, "fetchStart")))
	obj.Set("domainLookupStart", jsvalue.NewNumber(safeFloat(timingInfo, "domainLookupStart")))
	obj.Set("domainLookupEnd", jsvalue.NewNumber(safeFloat(timingInfo, "domainLookupEnd")))
	obj.Set("connectStart", jsvalue.NewNumber(safeFloat(timingInfo, "connectStart")))
	obj.Set("connectEnd", jsvalue.NewNumber(safeFloat(timingInfo, "connectEnd")))
	obj.Set("secureConnectionStart", jsvalue.NewNumber(safeFloat(timingInfo, "secureConnectionStart")))
	obj.Set("requestStart", jsvalue.NewNumber(safeFloat(timingInfo, "requestStart")))
	obj.Set("responseEnd", jsvalue.NewNumber(safeFloat(timingInfo, "responseEnd")))

	obj.Set("transferSize", jsvalue.NewNumber(safeFloat(timingInfo, "transferSize")))
	obj.Set("encodedBodySize", jsvalue.NewNumber(safeFloat(timingInfo, "encodedBodySize")))
	obj.Set("decodedBodySize", jsvalue.NewNumber(safeFloat(timingInfo, "decodedBodySize")))

	obj.Set("toJSON", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		copy := jsvalue.NewObject()
		copy.Set("name", jsvalue.NewString(e.name))
		copy.Set("entryType", jsvalue.NewString(e.entryType))
		copy.Set("startTime", jsvalue.NewNumber(e.startTime))
		copy.Set("duration", jsvalue.NewNumber(e.duration))
		copy.Set("workerStart", jsvalue.NewNumber(safeFloat(timingInfo, "workerStart")))
		copy.Set("redirectStart", jsvalue.NewNumber(safeFloat(timingInfo, "redirectStart")))
		copy.Set("redirectEnd", jsvalue.NewNumber(safeFloat(timingInfo, "redirectEnd")))
		copy.Set("fetchStart", jsvalue.NewNumber(safeFloat(timingInfo, "fetchStart")))
		copy.Set("domainLookupStart", jsvalue.NewNumber(safeFloat(timingInfo, "domainLookupStart")))
		copy.Set("domainLookupEnd", jsvalue.NewNumber(safeFloat(timingInfo, "domainLookupEnd")))
		copy.Set("connectStart", jsvalue.NewNumber(safeFloat(timingInfo, "connectStart")))
		copy.Set("connectEnd", jsvalue.NewNumber(safeFloat(timingInfo, "connectEnd")))
		copy.Set("secureConnectionStart", jsvalue.NewNumber(safeFloat(timingInfo, "secureConnectionStart")))
		copy.Set("requestStart", jsvalue.NewNumber(safeFloat(timingInfo, "requestStart")))
		copy.Set("responseEnd", jsvalue.NewNumber(safeFloat(timingInfo, "responseEnd")))
		copy.Set("transferSize", jsvalue.NewNumber(safeFloat(timingInfo, "transferSize")))
		copy.Set("encodedBodySize", jsvalue.NewNumber(safeFloat(timingInfo, "encodedBodySize")))
		copy.Set("decodedBodySize", jsvalue.NewNumber(safeFloat(timingInfo, "decodedBodySize")))
		return copy
	}))

	return obj
}
