package perf_hooks_test

import (
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/perf_hooks"
)

// resetEntries clears all marks, measures, and resource timings.
func resetEntries() {
	perf_hooks.ClearMarks(nil)
	perf_hooks.ClearMeasures(nil)
}

func TestNow(t *testing.T) {
	n1 := perf_hooks.PerformanceNow()
	if n1.Number() <= 0 {
		t.Fatal("now should be positive")
	}
	n2 := perf_hooks.PerformanceNow()
	if n2.Number() < n1.Number() {
		t.Fatal("now should be non-decreasing")
	}
}

func TestTimeOrigin(t *testing.T) {
	to := perf_hooks.TimeOrigin()
	if to.Number() <= 0 {
		t.Fatal("timeOrigin should be positive Unix timestamp")
	}
}

func TestMark(t *testing.T) {
	resetEntries()
	entry := perf_hooks.Mark(jsvalue.NewString("test"), nil)
	if entry == nil || entry.Type() == jsvalue.TypeUndefined {
		t.Fatal("mark should return an entry")
	}
	name := entry.Get("name")
	if name == nil || name.String() != "test" {
		t.Fatalf("expected name=test, got %v", name)
	}
	et := entry.Get("entryType")
	if et == nil || et.String() != "mark" {
		t.Fatalf("expected entryType=mark, got %v", et)
	}
	dur := entry.Get("duration")
	if dur == nil || dur.Number() != 0 {
		t.Fatalf("expected duration=0 for a mark, got %v", dur)
	}
	st := entry.Get("startTime")
	if st == nil || st.Number() <= 0 {
		t.Fatalf("expected startTime > 0, got %v", st)
	}
}

func TestMarkWithOptions(t *testing.T) {
	resetEntries()
	opts := jsvalue.NewObject()
	opts.Set("startTime", jsvalue.NewNumber(1234.5))
	opts.Set("detail", jsvalue.NewString("hello"))
	entry := perf_hooks.Mark(jsvalue.NewString("test2"), opts)
	if entry == nil || entry.Type() == jsvalue.TypeUndefined {
		t.Fatal("mark with options should return an entry")
	}
	st := entry.Get("startTime")
	if st == nil || st.Number() != 1234.5 {
		t.Fatalf("expected startTime=1234.5, got %v", st)
	}
	detail := entry.Get("detail")
	if detail == nil || detail.String() != "hello" {
		t.Fatalf("expected detail=hello, got %v", detail)
	}
}

func TestMeasure(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("A"), nil)
	time.Sleep(time.Millisecond)
	perf_hooks.Mark(jsvalue.NewString("B"), nil)
	m := perf_hooks.Measure(jsvalue.NewString("AB"), jsvalue.NewString("A"), jsvalue.NewString("B"))
	if m == nil || m.Type() == jsvalue.TypeUndefined {
		t.Fatal("measure should return an entry")
	}
	dur := m.Get("duration")
	if dur == nil || dur.Number() <= 0 {
		t.Fatalf("expected duration > 0 for measure A->B, got %v", dur)
	}
}

func TestClearMarks(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("m1"), nil)
	perf_hooks.Mark(jsvalue.NewString("m2"), nil)
	perf_hooks.ClearMarks(nil)
	marks := perf_hooks.GetEntriesByType(jsvalue.NewString("mark"))
	if marks == nil || marks.Len() != 0 {
		t.Fatalf("expected 0 marks after clearMarks(nil), got %d", marks.Len())
	}
}

func TestClearMarksByName(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("m1"), nil)
	perf_hooks.Mark(jsvalue.NewString("m2"), nil)
	perf_hooks.ClearMarks(jsvalue.NewString("m1"))
	marks := perf_hooks.GetEntriesByType(jsvalue.NewString("mark"))
	if marks == nil || marks.Len() != 1 {
		t.Fatalf("expected 1 mark after clearing m1, got %d", marks.Len())
	}
	arr := marks.Array()
	if len(arr) == 0 {
		t.Fatal("expected a mark entry")
	}
	name := arr[0].Get("name")
	if name == nil || name.String() != "m2" {
		t.Fatalf("expected remaining mark to be m2, got %v", name)
	}
}

func TestClearMeasures(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("A"), nil)
	time.Sleep(time.Millisecond)
	perf_hooks.Mark(jsvalue.NewString("B"), nil)
	perf_hooks.Measure(jsvalue.NewString("AB"), jsvalue.NewString("A"), jsvalue.NewString("B"))
	perf_hooks.ClearMeasures(nil)
	measures := perf_hooks.GetEntriesByType(jsvalue.NewString("measure"))
	if measures == nil || measures.Len() != 0 {
		t.Fatalf("expected 0 measures after clearMeasures(nil), got %d", measures.Len())
	}
}

func TestGetEntries(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("x1"), nil)
	perf_hooks.Mark(jsvalue.NewString("x2"), nil)
	perf_hooks.Measure(jsvalue.NewString("x1-x2"), jsvalue.NewString("x1"), jsvalue.NewString("x2"))
	all := perf_hooks.GetEntries()
	if all == nil || all.Len() != 3 {
		t.Fatalf("expected 3 entries total, got %d", all.Len())
	}
}

func TestGetEntriesByName(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("foo"), nil)
	perf_hooks.Mark(jsvalue.NewString("bar"), nil)
	foo := perf_hooks.GetEntriesByName(jsvalue.NewString("foo"), nil)
	if foo == nil || foo.Len() != 1 {
		t.Fatalf("expected 1 entry named foo, got %d", foo.Len())
	}
}

func TestGetEntriesByType(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("m1"), nil)
	perf_hooks.Mark(jsvalue.NewString("m2"), nil)
	perf_hooks.Measure(jsvalue.NewString("m1-m2"), jsvalue.NewString("m1"), jsvalue.NewString("m2"))
	marks := perf_hooks.GetEntriesByType(jsvalue.NewString("mark"))
	if marks == nil || marks.Len() != 2 {
		t.Fatalf("expected 2 marks, got %d", marks.Len())
	}
	measures := perf_hooks.GetEntriesByType(jsvalue.NewString("measure"))
	if measures == nil || measures.Len() != 1 {
		t.Fatalf("expected 1 measure, got %d", measures.Len())
	}
}

func TestPerformanceObserver(t *testing.T) {
	resetEntries()

	var received *jsvalue.JSValue
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			received = args[0]
		}
		return jsvalue.NewUndefined()
	})

	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	if ctor == nil {
		t.Fatal("PerformanceObserver constructor not found on exports")
	}
	observer := ctor.Call(callback)
	if observer == nil {
		t.Fatal("failed to create PerformanceObserver")
	}

	observeFn := observer.Get("observe")
	if observeFn == nil {
		t.Fatal("observer.observe not found")
	}
	opts := jsvalue.NewObject()
	opts.Set("entryTypes", jsvalue.NewArray(jsvalue.NewString("mark")))
	observeFn.Call(opts)

	perf_hooks.Mark(jsvalue.NewString("obs-test"), nil)

	if received == nil {
		t.Fatal("observer callback was not invoked")
	}
	if !received.IsArray() || received.Len() == 0 {
		t.Fatal("observer callback should receive a non-empty entry list")
	}
}

func TestPerformanceObserverDisconnect(t *testing.T) {
	resetEntries()

	var callCount int
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		callCount++
		return jsvalue.NewUndefined()
	})

	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	observer := ctor.Call(callback)

	observeFn := observer.Get("observe")
	opts := jsvalue.NewObject()
	opts.Set("entryTypes", jsvalue.NewArray(jsvalue.NewString("mark")))
	observeFn.Call(opts)

	perf_hooks.Mark(jsvalue.NewString("before-disconnect"), nil)
	if callCount != 1 {
		t.Fatalf("expected 1 callback before disconnect, got %d", callCount)
	}

	disconnectFn := observer.Get("disconnect")
	if disconnectFn == nil {
		t.Fatal("observer.disconnect not found")
	}
	disconnectFn.Call()

	perf_hooks.Mark(jsvalue.NewString("after-disconnect"), nil)
	if callCount != 1 {
		t.Fatalf("expected callback count to remain 1 after disconnect, got %d", callCount)
	}
}

func TestNodeTiming(t *testing.T) {
	timing := perf_hooks.GetNodeTiming()
	if timing == nil {
		t.Fatal("GetNodeTiming should return an object")
	}

	nodeStart := timing.Get("nodeStart")
	if nodeStart == nil || nodeStart.Number() != 0 {
		t.Fatalf("expected nodeStart=0, got %v", nodeStart)
	}

	loopStart := timing.Get("loopStart")
	if loopStart == nil || loopStart.Number() != -1 {
		t.Fatalf("expected loopStart=-1, got %v", loopStart)
	}

	bootstrapComplete := timing.Get("bootstrapComplete")
	if bootstrapComplete == nil || bootstrapComplete.Number() <= 0 {
		t.Fatalf("expected bootstrapComplete > 0, got %v", bootstrapComplete)
	}
}

func TestEventLoopUtilization(t *testing.T) {
	elu := perf_hooks.EventLoopUtilization()
	if elu == nil {
		t.Fatal("EventLoopUtilization should return an object")
	}

	idle := elu.Get("idle")
	if idle == nil {
		t.Fatal("elu should have idle property")
	}

	active := elu.Get("active")
	if active == nil {
		t.Fatal("elu should have active property")
	}

	util := elu.Get("utilization")
	if util == nil {
		t.Fatal("elu should have utilization property")
	}

	u := util.Number()
	if u < 0 || u > 1 {
		t.Fatalf("expected utilization between 0 and 1, got %f", u)
	}
}

func TestCreateHistogram(t *testing.T) {
	hist := perf_hooks.CreateHistogram()
	if hist == nil {
		t.Fatal("CreateHistogram should return an object")
	}

	countFn := hist.Get("count")
	if countFn == nil {
		t.Fatal("histogram should have count method")
	}
	count := countFn.Call()
	if count == nil || count.Number() != 0 {
		t.Fatalf("expected count=0 for new histogram, got %v", count)
	}

	minFn := hist.Get("min")
	min := minFn.Call()
	if min == nil || min.Number() != 0 {
		t.Fatalf("expected min=0 for new histogram, got %v", min)
	}

	maxFn := hist.Get("max")
	max := maxFn.Call()
	if max == nil || max.Number() != 0 {
		t.Fatalf("expected max=0 for new histogram, got %v", max)
	}

	recordFn := hist.Get("record")
	if recordFn == nil {
		t.Fatal("histogram should have record method")
	}
	recordFn.Call(jsvalue.NewNumber(42.5))

	count = countFn.Call()
	if count == nil || count.Number() != 1 {
		t.Fatalf("expected count=1 after recording, got %v", count)
	}

	min = minFn.Call()
	if min == nil || min.Number() != 42.5 {
		t.Fatalf("expected min=42.5 after recording, got %v", min)
	}

	max = maxFn.Call()
	if max == nil || max.Number() != 42.5 {
		t.Fatalf("expected max=42.5 after recording, got %v", max)
	}
}

func TestAsJSValueExports(t *testing.T) {
	exports := perf_hooks.AsJSValue
	if exports == nil {
		t.Fatal("AsJSValue should return an object")
	}

	perf := exports.Get("performance")
	if perf == nil {
		t.Fatal("exports should have 'performance' key")
	}

	constants := exports.Get("constants")
	if constants == nil {
		t.Fatal("exports should have 'constants' key")
	}

	expectedPerfKeys := []string{"now", "mark", "measure", "clearMarks", "clearMeasures", "getEntries", "timeOrigin", "nodeTiming"}
	for _, key := range expectedPerfKeys {
		v := perf.Get(key)
		if v == nil {
			t.Errorf("performance should have '%s' key", key)
		}
	}
}

func TestConstants(t *testing.T) {
	constants := perf_hooks.GetConstants()
	if constants == nil {
		t.Fatal("GetConstants should return an object")
	}

	major := constants.Get("NODE_PERFORMANCE_GC_MAJOR")
	if major == nil || major.Number() != 1 {
		t.Fatalf("expected NODE_PERFORMANCE_GC_MAJOR=1, got %v", major)
	}

	minor := constants.Get("NODE_PERFORMANCE_GC_MINOR")
	if minor == nil || minor.Number() != 2 {
		t.Fatalf("expected NODE_PERFORMANCE_GC_MINOR=2, got %v", minor)
	}

	incremental := constants.Get("NODE_PERFORMANCE_GC_INCREMENTAL")
	if incremental == nil || incremental.Number() != 4 {
		t.Fatalf("expected NODE_PERFORMANCE_GC_INCREMENTAL=4, got %v", incremental)
	}

	weakcb := constants.Get("NODE_PERFORMANCE_GC_WEAKCB")
	if weakcb == nil || weakcb.Number() != 8 {
		t.Fatalf("expected NODE_PERFORMANCE_GC_WEAKCB=8, got %v", weakcb)
	}
}

func TestToJSON(t *testing.T) {
	resetEntries()
	perf_hooks.Mark(jsvalue.NewString("json-test"), nil)
	result := perf_hooks.ToJSON()
	if result == nil {
		t.Fatal("ToJSON should return an object")
	}

	timeOrigin := result.Get("timeOrigin")
	if timeOrigin == nil {
		t.Fatal("ToJSON result should have timeOrigin property")
	}

	entriesList := result.Get("entries")
	if entriesList == nil {
		t.Fatal("ToJSON result should have entries property")
	}
	if !entriesList.IsArray() {
		t.Fatal("ToJSON entries should be an array")
	}
	if entriesList.Len() != 1 {
		t.Fatalf("expected 1 entry in ToJSON, got %d", entriesList.Len())
	}
}

// --- Error behavior tests ---

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic but function did not throw")
		}
	}()
	fn()
}

func TestMarkNoArgs(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Mark(nil, nil)
	})
}

func TestMarkUndefined(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Mark(jsvalue.NewUndefined(), nil)
	})
}

func TestMarkEmptyName(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Mark(jsvalue.NewString(""), nil)
	})
}

func TestMarkNonString(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Mark(jsvalue.NewNumber(42), nil)
	})
}

func TestMeasureNoArgs(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Measure(nil, nil, nil)
	})
}

func TestMeasureNonStringName(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Measure(jsvalue.NewNumber(42), nil, nil)
	})
}

func TestMeasureNonexistentMark(t *testing.T) {
	resetEntries()
	expectPanic(t, func() {
		perf_hooks.Measure(jsvalue.NewString("test"), jsvalue.NewString("nonexistent"), nil)
	})
}

func TestPerformanceObserverNoCallback(t *testing.T) {
	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	expectPanic(t, func() {
		ctor.Call()
	})
}

func TestPerformanceObserverNonFunctionCallback(t *testing.T) {
	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	expectPanic(t, func() {
		ctor.Call(jsvalue.NewString("not a function"))
	})
}

func TestObserverObserveNoArgs(t *testing.T) {
	resetEntries()
	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
	obs := ctor.Call(cb)
	observeFn := obs.Get("observe")
	expectPanic(t, func() {
		observeFn.Call()
	})
}

func TestObserverObserveEmptyEntryTypes(t *testing.T) {
	resetEntries()
	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
	obs := ctor.Call(cb)
	observeFn := obs.Get("observe")
	opts := jsvalue.NewObject()
	opts.Set("entryTypes", jsvalue.NewArray())
	expectPanic(t, func() {
		observeFn.Call(opts)
	})
}

func TestObserverObserveInvalidType(t *testing.T) {
	resetEntries()
	exports := perf_hooks.AsJSValue
	ctor := exports.Get("PerformanceObserver")
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() })
	obs := ctor.Call(cb)
	observeFn := obs.Get("observe")
	opts := jsvalue.NewObject()
	opts.Set("type", jsvalue.NewString("invalid"))
	expectPanic(t, func() {
		observeFn.Call(opts)
	})
}
