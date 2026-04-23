package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// newTestLoop creates a fresh EventLoop for testing.
func newTestLoop() *EventLoop {
	return &EventLoop{
		jobChan:    make(chan func(), 256),
		wakeupChan: make(chan struct{}, 1),
	}
}

func TestSetTimeoutFires(t *testing.T) {
	el := newTestLoop()

	var called atomic.Bool
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called.Store(true)
		return jsvalue.NewUndefined()
	})

	el.scheduleTimeout(10, func() { callback.Call() }, 1)

	go el.Run()

	time.Sleep(100 * time.Millisecond)
	if !called.Load() {
		t.Fatal("setTimeout callback did not fire")
	}
}

func TestSetTimeoutCancel(t *testing.T) {
	el := newTestLoop()

	var called atomic.Bool
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called.Store(true)
		return jsvalue.NewUndefined()
	})

	id := el.nextJobID.Add(1)
	el.scheduleTimeout(50, func() { callback.Call() }, id)
	ClearTimeout(jsvalue.NewNumber(float64(id)))

	time.Sleep(150 * time.Millisecond)
	if called.Load() {
		t.Fatal("setTimeout callback fired after clearTimeout")
	}
}

func TestSetIntervalFires(t *testing.T) {
	el := newTestLoop()

	var count atomic.Int32
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		count.Add(1)
		return jsvalue.NewUndefined()
	})

	id := el.nextJobID.Add(1)
	el.scheduleInterval(20, func() { callback.Call() }, id)

	go el.Run()

	time.Sleep(120 * time.Millisecond)
	el.cancelJob(int64(id))

	if c := count.Load(); c < 3 {
		t.Fatalf("setInterval fired %d times, expected at least 3", c)
	}
}

func TestSetIntervalCancel(t *testing.T) {
	el := newTestLoop()

	var count atomic.Int32
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		count.Add(1)
		return jsvalue.NewUndefined()
	})

	id := el.nextJobID.Add(1)
	el.scheduleInterval(20, func() { callback.Call() }, id)

	go el.Run()

	time.Sleep(80 * time.Millisecond)
	el.cancelJob(int64(id))
	countAtCancel := count.Load()

	time.Sleep(100 * time.Millisecond)
	countAfter := count.Load()

	if countAfter > countAtCancel+1 {
		t.Fatalf("setInterval kept firing after clearInterval: before=%d after=%d", countAtCancel, countAfter)
	}
}

func TestSetImmediateFires(t *testing.T) {
	el := newTestLoop()

	var called atomic.Bool
	callback := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called.Store(true)
		return jsvalue.NewUndefined()
	})

	el.scheduleImmediate(func() { callback.Call() })

	go el.Run()

	time.Sleep(50 * time.Millisecond)
	if !called.Load() {
		t.Fatal("setImmediate callback did not fire")
	}
}

func TestHandleKeepsAlive(t *testing.T) {
	el := newTestLoop()
	el.RegisterHandle()

	done := make(chan struct{})
	go func() {
		el.Run()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Run() returned while handle was active")
	case <-time.After(200 * time.Millisecond):
		// expected: still running
	}

	el.UnregisterHandle()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return after UnregisterHandle")
	}
}

func TestScheduleCallbackRunsOnLoop(t *testing.T) {
	el := newTestLoop()
	el.RegisterHandle()

	done := make(chan struct{})
	go func() {
		el.Run()
		close(done)
	}()

	var called atomic.Bool
	go func() {
		time.Sleep(20 * time.Millisecond)
		el.ScheduleCallback(func() {
			called.Store(true)
			el.UnregisterHandle()
		})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after scheduled callback completed")
	}

	if !called.Load() {
		t.Fatal("scheduled callback did not run")
	}
}

func TestProcessExitsWhenIdle(t *testing.T) {
	el := newTestLoop()

	done := make(chan struct{})
	go func() {
		el.Run()
		close(done)
	}()

	select {
	case <-done:
		// expected: returns immediately
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() blocked when idle")
	}
}

func TestMultipleTimers(t *testing.T) {
	el := newTestLoop()

	var mu sync.Mutex
	results := []int{}

	makeCallback := func(n int) func() {
		return func() {
			mu.Lock()
			results = append(results, n)
			mu.Unlock()
		}
	}

	el.scheduleTimeout(10, makeCallback(1), 1)
	el.scheduleTimeout(20, makeCallback(2), 2)
	el.scheduleTimeout(30, makeCallback(3), 3)

	go el.Run()

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(results), results)
	}
}

func TestCancelFireRace(t *testing.T) {
	el := newTestLoop()

	var fired atomic.Int32
	callback := func() { fired.Add(1) }

	go el.Run()

	const n = 100
	ids := make([]int64, n)
	for i := 0; i < n; i++ {
		id := el.nextJobID.Add(1)
		ids[i] = id
		el.scheduleTimeout(1, callback, id)
	}

	// Cancel half immediately
	for i := 0; i < n/2; i++ {
		el.cancelJob(ids[i])
	}

	time.Sleep(200 * time.Millisecond)

	// jobCount should be 0 (all either fired or cancelled)
	if jc := el.jobCount.Load(); jc != 0 {
		t.Fatalf("jobCount=%d, expected 0 (negative means underflow race)", jc)
	}
}

func TestTimerPanicRecovery(t *testing.T) {
	el := newTestLoop()

	panicFn := func() { panic("test panic") }

	var afterCalled atomic.Bool
	afterFn := func() { afterCalled.Store(true) }

	el.scheduleTimeout(10, panicFn, 1)
	el.scheduleTimeout(20, afterFn, 2)

	go el.Run()

	time.Sleep(100 * time.Millisecond)

	if !afterCalled.Load() {
		t.Fatal("second timer did not fire after first timer panicked")
	}
}
