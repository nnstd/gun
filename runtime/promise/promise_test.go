package promise

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

// runLoop starts the Default event loop in a goroutine and returns a wait
// function that blocks until the loop exits or the timeout fires.
func runLoop(t *testing.T) func() {
	t.Helper()
	done := make(chan struct{})
	go func() {
		eventloop.Default.Run()
		close(done)
	}()
	return func() {
		t.Helper()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("event loop did not exit within timeout")
		}
	}
}

// --- Basic async resolution ---

func TestAsyncResolveThen(t *testing.T) {
	var got string
	handlerDone := make(chan struct{})

	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			got = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for .then handler")
	}
	wait()

	if got != "ok" {
		t.Fatalf("resolved value = %q, want ok", got)
	}
}

func TestAsyncRejectCatch(t *testing.T) {
	handlerDone := make(chan struct{})

	p := Promise.Get("reject").Call(jsvalue.NewString("bad"))
	p.MethodCall("catch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(handlerDone)
		return jsvalue.NewString("handled")
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for .catch handler")
	}
	wait()
}

func TestAsyncFinally(t *testing.T) {
	var called atomic.Bool
	handlerDone := make(chan struct{})

	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	p.MethodCall("finally", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called.Store(true)
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for .finally handler")
	}
	wait()

	if !called.Load() {
		t.Fatal("finally callback not called")
	}
}

// --- Chaining ---

func TestAsyncChainedThen(t *testing.T) {
	var got float64
	handlerDone := make(chan struct{})

	p := Promise.Get("resolve").Call(jsvalue.NewNumber(1))
	p1 := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewNumber(args[0].Number() + 10)
		}
		return jsvalue.NewUndefined()
	}))
	p1.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].Number() * 2
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for chained .then")
	}
	wait()

	if got != 22 { // (1+10)*2
		t.Fatalf("chained result = %v, want 22", got)
	}
}

// --- Error propagation ---

func TestAsyncThenErrorPropagation(t *testing.T) {
	handlerDone := make(chan struct{})
	var gotReason string

	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		panic("callback threw")
	}))
	next.MethodCall("then",
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		}),
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			gotReason = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
		}),
	)

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error propagation")
	}
	wait()

	if gotReason != "callback threw" {
		t.Fatalf("rejection reason = %q, want 'callback threw'", gotReason)
	}
}

// --- Promise.all ---

func TestAsyncPromiseAll(t *testing.T) {
	handlerDone := make(chan struct{})
	var gotLen int

	p1 := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(10 * time.Millisecond)
			resolve.Call(jsvalue.NewString("a"))
		}()
		return jsvalue.NewUndefined()
	}))
	p2 := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(20 * time.Millisecond)
			resolve.Call(jsvalue.NewString("b"))
		}()
		return jsvalue.NewUndefined()
	}))

	arr := jsvalue.NewArray(p1, p2)
	result := Promise.Get("all").Call(arr)
	result.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			gotLen = args[0].Len()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Promise.all")
	}
	wait()

	if gotLen != 2 {
		t.Fatalf("Promise.all result len = %d, want 2", gotLen)
	}
}

// --- Promise.race ---

func TestAsyncPromiseRace(t *testing.T) {
	handlerDone := make(chan struct{})
	var got string

	p1 := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(50 * time.Millisecond)
			resolve.Call(jsvalue.NewString("slow"))
		}()
		return jsvalue.NewUndefined()
	}))
	p2 := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(10 * time.Millisecond)
			resolve.Call(jsvalue.NewString("fast"))
		}()
		return jsvalue.NewUndefined()
	}))

	arr := jsvalue.NewArray(p1, p2)
	result := Promise.Get("race").Call(arr)
	result.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Promise.race")
	}
	wait()

	if got != "fast" {
		t.Fatalf("Promise.race result = %q, want fast", got)
	}
}

// --- Event loop liveness ---

func TestAsyncEventLoopLiveness(t *testing.T) {
	handlerDone := make(chan struct{})

	p := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(100 * time.Millisecond)
			resolve.Call(jsvalue.NewNumber(42))
		}()
		return jsvalue.NewUndefined()
	}))
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	start := time.Now()
	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout — event loop exited prematurely?")
	}
	wait()
	elapsed := time.Since(start)

	if elapsed < 80*time.Millisecond {
		t.Fatalf("elapsed = %v, expected at least 80ms (promise resolved after 100ms)", elapsed)
	}
}

// --- Panic recovery ---

func TestAsyncPanicRecovery(t *testing.T) {
	handlerDone := make(chan struct{})
	var gotReason string

	p2 := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		panic("executor panicked")
	}))
	p2.MethodCall("then",
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() }),
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				gotReason = args[0].String()
			}
			close(handlerDone)
			return jsvalue.NewUndefined()
		}),
	)

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for panic recovery")
	}
	wait()

	if gotReason != "executor panicked" {
		t.Fatalf("panic recovery reason = %q, want 'executor panicked'", gotReason)
	}
}

// --- Mixed timers + promises ---

func TestAsyncMixedTimersAndPromises(t *testing.T) {
	handlerDone := make(chan struct{})
	var order []string
	var mu sync.Mutex

	addOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	// Promise resolves first
	p := Promise.Get("resolve").Call(jsvalue.NewString("promise"))
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		addOrder("promise")
		return jsvalue.NewUndefined()
	}))

	// Timer fires later
	eventloop.Default.ScheduleMicrotask(func() {
		addOrder("microtask")
		close(handlerDone)
	})

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	wait()

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 {
		t.Fatalf("order = %v, expected at least 2 entries", order)
	}
}

// --- Await blocks ---

func TestAwaitBlocksForPendingPromise(t *testing.T) {
	p := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		resolve := args[0]
		go func() {
			time.Sleep(50 * time.Millisecond)
			resolve.Call(jsvalue.NewString("delayed"))
		}()
		return jsvalue.NewUndefined()
	}))

	go eventloop.Default.Run()

	result := Await(p)
	if result.String() != "delayed" {
		t.Fatalf("Await result = %q, want delayed", result.String())
	}

	// Wait for event loop to finish
	time.Sleep(50 * time.Millisecond)
}

func TestAwaitReturnsImmediatelyForResolved(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewString("instant"))
	result := Await(p)
	if result.String() != "instant" {
		t.Fatalf("Await result = %q, want instant", result.String())
	}
}

func TestAwaitNonPromise(t *testing.T) {
	v := jsvalue.NewString("plain")
	result := Await(v)
	if result.String() != "plain" {
		t.Fatalf("Await(nonPromise) = %q, want plain", result.String())
	}
}

// --- Concurrent fulfill ---

func TestConcurrentFulfill(t *testing.T) {
	p := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))

	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pi := getPromiseInternal(p)
			if pi != nil {
				pi.mu.Lock()
				if getState(p) == statePending {
					defineInternal(p, promiseStateKey, jsvalue.NewString(stateFulfilled))
					defineInternal(p, promiseValueKey, jsvalue.NewNumber(42))
					close(pi.settled)
					promiseInternals.Delete(p)
					pi.mu.Unlock()
					successes.Add(1)
					dispatchHandlers(p)
					eventloop.Default.SettlePromise()
					return
				}
				pi.mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("concurrent fulfill successes = %d, want exactly 1", successes.Load())
	}
}

// --- Internal slots not enumerable ---

func TestAsyncInternalSlotsAreNotEnumerable(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	if got := jsvalue.Keys(p).Len(); got != 0 {
		t.Fatalf("Object.keys(promise) len = %d, want 0", got)
	}
}

// --- ResolvedPromise helper ---

func TestResolvedPromiseState(t *testing.T) {
	p := ResolvedPromise(jsvalue.NewString("immediate"))
	if getState(p) != stateFulfilled {
		t.Fatalf("state = %q, want fulfilled", getState(p))
	}
	if p.Get(promiseValueKey).String() != "immediate" {
		t.Fatalf("value = %q, want immediate", p.Get(promiseValueKey).String())
	}
}

func TestResolvedPromiseThen(t *testing.T) {
	handlerDone := make(chan struct{})
	var got float64

	p := ResolvedPromise(jsvalue.NewNumber(99))
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].Number() + 1
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	wait()

	if got != 100 {
		t.Fatalf("ResolvedPromise.then result = %v, want 100", got)
	}
}

func TestResolvedPromiseThenChained(t *testing.T) {
	handlerDone := make(chan struct{})
	var got string

	p := ResolvedPromise(jsvalue.NewString("hello"))
	p1 := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewString(args[0].String() + " world")
		}
		return jsvalue.NewUndefined()
	}))
	p1.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	wait()

	if got != "hello world" {
		t.Fatalf("chained ResolvedPromise.then = %q, want 'hello world'", got)
	}
}

// --- Thenable adoption ---

func TestAsyncResolveAdoptsThenable(t *testing.T) {
	handlerDone := make(chan struct{})
	var got string

	thenable := jsvalue.ObjectFrom(
		"then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				return args[0].Call(jsvalue.NewString("thenable-ok"))
			}
			return jsvalue.NewUndefined()
		}),
	)
	p := Promise.Get("resolve").Call(thenable)
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for thenable adoption")
	}
	wait()

	if got != "thenable-ok" {
		t.Fatalf("thenable adoption result = %q, want thenable-ok", got)
	}
}

// --- ResolvedPromise thenable unwrap ---

func TestResolvedPromiseThenThenableUnwrap(t *testing.T) {
	handlerDone := make(chan struct{})
	var got string

	inner := ResolvedPromise(jsvalue.NewString("inner-value"))
	p := ResolvedPromise(jsvalue.NewString("outer"))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return inner
	}))
	next.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			got = args[0].String()
		}
		close(handlerDone)
		return jsvalue.NewUndefined()
	}))

	wait := runLoop(t)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	wait()

	if got != "inner-value" {
		t.Fatalf("thenable unwrap = %q, want inner-value", got)
	}
}

// --- TOCTOU race stress test ---

// TestThenRaceStress exercises the TOCTOU race between .then() and fulfill().
// Before the fix, .then() could read PENDING and push a handler while fulfill()
// was concurrently transitioning state and clearing handler arrays. This test
// runs 500 iterations with concurrent resolve + .then() to detect the race.
func TestThenRaceStress(t *testing.T) {
	const iterations = 500
	var failures atomic.Int32

	for i := 0; i < iterations; i++ {
		p := Promise.Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		}))

		var wg sync.WaitGroup
		var handlerCalled atomic.Int32

		// Racer 1: resolve the promise
		wg.Add(1)
		go func() {
			defer wg.Done()
			fulfill(p, jsvalue.NewString("resolved"))
		}()

		// Racer 2: attach .then() handler
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
				handlerCalled.Add(1)
				return jsvalue.NewUndefined()
			}))
		}()

		wg.Wait()

		// The .then() handler must eventually fire via the event loop.
		// Give it a chance by running a microtask cycle.
		done := make(chan struct{})
		eventloop.Default.ScheduleMicrotask(func() {
			close(done)
		})

		wait := runLoop(t)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for handler microtask")
		}
		wait()

		if handlerCalled.Load() != 1 {
			failures.Add(1)
		}
	}

	if failures.Load() > 0 {
		t.Fatalf("TOCTOU race detected: %d/%d iterations lost .then() handler", failures.Load(), iterations)
	}
}
