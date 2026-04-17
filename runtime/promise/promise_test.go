package promise

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestPromiseResolveThen(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	got := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return args[0]
		}
		return jsvalue.NewUndefined()
	}))
	if got.Get(promiseValueKey).String() != "ok" {
		t.Fatalf("resolved value = %q, want ok", got.Get(promiseValueKey).String())
	}
}

func TestPromiseRejectCatch(t *testing.T) {
	p := Promise.Get("reject").Call(jsvalue.NewString("bad"))
	got := p.MethodCall("catch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString("handled")
	}))
	if got.Get(promiseValueKey).String() != "handled" {
		t.Fatalf("catch result = %q, want handled", got.Get(promiseValueKey).String())
	}
}

func TestPromiseFinally(t *testing.T) {
	called := false
	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	got := p.MethodCall("finally", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called = true
		return jsvalue.NewUndefined()
	}))
	if !called {
		t.Fatal("finally callback not called")
	}
	if got.Get(promiseValueKey).String() != "ok" {
		t.Fatalf("finally result = %q, want ok", got.Get(promiseValueKey).String())
	}
}

func TestPromiseAll(t *testing.T) {
	arr := jsvalue.NewArray(
		Promise.Get("resolve").Call(jsvalue.NewString("a")),
		Promise.Get("resolve").Call(jsvalue.NewString("b")),
	)
	got := Promise.Get("all").Call(arr)
	if got.Get(promiseValueKey).Len() != 2 {
		t.Fatalf("Promise.all len = %d, want 2", got.Get(promiseValueKey).Len())
	}
}

func TestPromiseInternalSlotsAreNotEnumerable(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	if got := jsvalue.Keys(p).Len(); got != 0 {
		t.Fatalf("Object.keys(promise) len = %d, want 0", got)
	}
}

func TestPromiseResolveAdoptsThenable(t *testing.T) {
	thenable := jsvalue.ObjectFrom(
		"then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				return args[0].Call(jsvalue.NewString("thenable-ok"))
			}
			return jsvalue.NewUndefined()
		}),
	)
	p := Promise.Get("resolve").Call(thenable)
	if got := p.Get(promiseValueKey).String(); got != "thenable-ok" {
		t.Fatalf("Promise.resolve(thenable) = %q, want thenable-ok", got)
	}
}

// --- Fast path tests ---

func TestThenFastPathResolvedPromise(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewNumber(42))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewNumber(args[0].Number() + 1)
		}
		return jsvalue.NewUndefined()
	}))
	if getState(next) != stateFulfilled {
		t.Fatalf("next state = %q, want fulfilled", getState(next))
	}
	if next.Get(promiseValueKey).Number() != 43 {
		t.Fatalf("next value = %v, want 43", next.Get(promiseValueKey).Number())
	}
}

func TestThenFastPathRejectedPromise(t *testing.T) {
	p := Promise.Get("reject").Call(jsvalue.NewString("err"))
	next := p.MethodCall("then",
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewString("should not run")
		}),
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				return jsvalue.NewString("caught: " + args[0].String())
			}
			return jsvalue.NewUndefined()
		}),
	)
	if getState(next) != stateFulfilled {
		t.Fatalf("next state = %q, want fulfilled", getState(next))
	}
	if next.Get(promiseValueKey).String() != "caught: err" {
		t.Fatalf("next value = %q, want 'caught: err'", next.Get(promiseValueKey).String())
	}
}

func TestThenFastPathErrorPropagation(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewString("ok"))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		panic("callback threw")
	}))
	if getState(next) != stateRejected {
		t.Fatalf("next state = %q, want rejected", getState(next))
	}
	reason := next.Get(promiseValueKey).String()
	if reason != "callback threw" {
		t.Fatalf("rejection reason = %q, want 'callback threw'", reason)
	}
}

func TestThenFastPathChained(t *testing.T) {
	p := Promise.Get("resolve").Call(jsvalue.NewNumber(1))
	p1 := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewNumber(args[0].Number() + 10)
		}
		return jsvalue.NewUndefined()
	}))
	p2 := p1.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewNumber(args[0].Number() * 2)
		}
		return jsvalue.NewUndefined()
	}))
	if getState(p2) != stateFulfilled {
		t.Fatalf("p2 state = %q, want fulfilled", getState(p2))
	}
	if p2.Get(promiseValueKey).Number() != 22 {
		t.Fatalf("p2 value = %v, want 22", p2.Get(promiseValueKey).Number())
	}
}

func TestThenFastPathFinallyAfterResolved(t *testing.T) {
	called := false
	p := Promise.Get("resolve").Call(jsvalue.NewString("yes"))
	next := p.MethodCall("finally", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called = true
		return jsvalue.NewUndefined()
	}))
	if !called {
		t.Fatal("finally callback not called in fast path")
	}
	if getState(next) != stateFulfilled {
		t.Fatalf("next state = %q, want fulfilled", getState(next))
	}
	if next.Get(promiseValueKey).String() != "yes" {
		t.Fatalf("next value = %q, want yes", next.Get(promiseValueKey).String())
	}
}

// --- ResolvedPromise helper tests ---

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
	p := ResolvedPromise(jsvalue.NewNumber(99))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewNumber(args[0].Number() + 1)
		}
		return jsvalue.NewUndefined()
	}))
	if getState(next) != stateFulfilled {
		t.Fatalf("next state = %q, want fulfilled", getState(next))
	}
	if next.Get(promiseValueKey).Number() != 100 {
		t.Fatalf("next value = %v, want 100", next.Get(promiseValueKey).Number())
	}
}

func TestResolvedPromiseDoubleThen(t *testing.T) {
	p := ResolvedPromise(jsvalue.NewString("hello"))
	p1 := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewString(args[0].String() + " world")
		}
		return jsvalue.NewUndefined()
	}))
	p2 := p1.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			return jsvalue.NewString(args[0].String() + "!")
		}
		return jsvalue.NewUndefined()
	}))
	if p2.Get(promiseValueKey).String() != "hello world!" {
		t.Fatalf("double then = %q, want 'hello world!'", p2.Get(promiseValueKey).String())
	}
}

func TestResolvedPromiseThenThenableUnwrap(t *testing.T) {
	inner := ResolvedPromise(jsvalue.NewString("inner-value"))
	p := ResolvedPromise(jsvalue.NewString("outer"))
	next := p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return inner
	}))
	// fulfill() should unwrap the thenable returned by onFulfilled
	if getState(next) != stateFulfilled {
		t.Fatalf("next state = %q, want fulfilled", getState(next))
	}
	if next.Get(promiseValueKey).String() != "inner-value" {
		t.Fatalf("next value = %q, want inner-value", next.Get(promiseValueKey).String())
	}
}
