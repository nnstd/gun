package perf_hooks

import (
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

const (
	NODE_PERFORMANCE_GC_MAJOR       = 1
	NODE_PERFORMANCE_GC_MINOR       = 2
	NODE_PERFORMANCE_GC_INCREMENTAL = 4
	NODE_PERFORMANCE_GC_WEAKCB      = 8

	NODE_PERFORMANCE_GC_FLAGS_NO                          = 0
	NODE_PERFORMANCE_GC_FLAGS_CONSTRUCT_RETAINED           = 1
	NODE_PERFORMANCE_GC_FLAGS_FORCED                       = 2
	NODE_PERFORMANCE_GC_FLAGS_SYNCHRONOUS_PHANTOM_PROCESSING = 4
	NODE_PERFORMANCE_GC_FLAGS_ALL_AVAILABLE_GARBAGE        = 8
	NODE_PERFORMANCE_GC_FLAGS_ALL_EXTERNAL_MEMORY          = 16
	NODE_PERFORMANCE_GC_FLAGS_SCHEDULE_IDLE                = 32
)

func Now() float64 {
	return float64(time.Now().UnixNano()) / 1e6
}

func createNodeEntry(name, entryType string, startTime, duration float64, detail *jsvalue.JSValue, flags, kind float64) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString(name))
	obj.Set("entryType", jsvalue.NewString(entryType))
	obj.Set("startTime", jsvalue.NewNumber(startTime))
	obj.Set("duration", jsvalue.NewNumber(duration))
	obj.Set("detail", detail)
	obj.Set("flags", jsvalue.NewNumber(flags))
	obj.Set("kind", jsvalue.NewNumber(kind))
	return obj
}

func NewGCEntry(kind, flags int) *jsvalue.JSValue {
	detail := jsvalue.NewObject()
	detail.Set("kind", jsvalue.NewNumber(float64(kind)))
	detail.Set("flags", jsvalue.NewNumber(float64(flags)))
	return createNodeEntry("gc", "gc", Now(), 0, detail, float64(flags), float64(kind))
}

func NewHTTPEntry(name string, req, res *jsvalue.JSValue) *jsvalue.JSValue {
	detail := jsvalue.NewObject()
	detail.Set("req", req)
	detail.Set("res", res)
	return createNodeEntry(name, "http", Now(), 0, detail, 0, 0)
}

func NewHTTP2Entry(name string, detail *jsvalue.JSValue) *jsvalue.JSValue {
	return createNodeEntry(name, "http2", Now(), 0, detail, 0, 0)
}

func NewFunctionEntry(name string, args []*jsvalue.JSValue, duration float64) *jsvalue.JSValue {
	return createNodeEntry(name, "function", 0, duration, jsvalue.NewArray(args...), 0, 0)
}

func NewNetEntry(name string, host string, port interface{}) *jsvalue.JSValue {
	detail := jsvalue.NewObject()
	detail.Set("host", jsvalue.NewString(host))
	switch v := port.(type) {
	case int:
		detail.Set("port", jsvalue.NewNumber(float64(v)))
	case float64:
		detail.Set("port", jsvalue.NewNumber(v))
	case *jsvalue.JSValue:
		detail.Set("port", v)
	default:
		detail.Set("port", jsvalue.NewNumber(0))
	}
	return createNodeEntry(name, "net", Now(), 0, detail, 0, 0)
}

func NewDNSEntry(name string, detail *jsvalue.JSValue) *jsvalue.JSValue {
	return createNodeEntry(name, "dns", Now(), 0, detail, 0, 0)
}

func GetConstants() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("NODE_PERFORMANCE_GC_MAJOR", jsvalue.NewNumber(NODE_PERFORMANCE_GC_MAJOR))
	obj.Set("NODE_PERFORMANCE_GC_MINOR", jsvalue.NewNumber(NODE_PERFORMANCE_GC_MINOR))
	obj.Set("NODE_PERFORMANCE_GC_INCREMENTAL", jsvalue.NewNumber(NODE_PERFORMANCE_GC_INCREMENTAL))
	obj.Set("NODE_PERFORMANCE_GC_WEAKCB", jsvalue.NewNumber(NODE_PERFORMANCE_GC_WEAKCB))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_NO", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_NO))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_CONSTRUCT_RETAINED", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_CONSTRUCT_RETAINED))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_FORCED", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_FORCED))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_SYNCHRONOUS_PHANTOM_PROCESSING", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_SYNCHRONOUS_PHANTOM_PROCESSING))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_ALL_AVAILABLE_GARBAGE", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_ALL_AVAILABLE_GARBAGE))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_ALL_EXTERNAL_MEMORY", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_ALL_EXTERNAL_MEMORY))
	obj.Set("NODE_PERFORMANCE_GC_FLAGS_SCHEDULE_IDLE", jsvalue.NewNumber(NODE_PERFORMANCE_GC_FLAGS_SCHEDULE_IDLE))
	return obj
}
