package perf_hooks

import (
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var (
	loopStartTime float64 = -1
	loopExitTime  float64 = -1
	bootstrapTime float64
	eluIdleNs     int64
	eluActiveNs   int64
	eluLastCheck  time.Time
)

func init() {
	bootstrapTime = Now()
	eluLastCheck = time.Now()
}

func MarkLoopStart() {
	loopStartTime = Now()
}

func MarkLoopExit() {
	loopExitTime = Now()
}

func UpdateELUMetrics(idleNs, activeNs int64) {
	eluIdleNs += idleNs
	eluActiveNs += activeNs
}

func GetNodeTiming() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString("node"))
	obj.Set("entryType", jsvalue.NewString("node"))
	obj.Set("startTime", jsvalue.NewNumber(0))
	obj.Set("duration", jsvalue.NewNumber(Now()))
	obj.Set("bootstrapComplete", jsvalue.NewNumber(bootstrapTime))
	obj.Set("environment", jsvalue.NewNumber(bootstrapTime))
	obj.Set("nodeStart", jsvalue.NewNumber(0))
	obj.Set("v8Start", jsvalue.NewNumber(0))
	obj.Set("loopStart", jsvalue.NewNumber(loopStartTime))
	obj.Set("loopExit", jsvalue.NewNumber(loopExitTime))
	obj.Set("idleTime", jsvalue.NewNumber(float64(eluIdleNs)/1e6))

	uvMetrics := jsvalue.NewObject()
	uvMetrics.Set("loopCount", jsvalue.NewNumber(0))
	uvMetrics.Set("events", jsvalue.NewNumber(0))
	uvMetrics.Set("eventsWaiting", jsvalue.NewNumber(0))
	obj.Set("uvMetricsInfo", uvMetrics)

	return obj
}
