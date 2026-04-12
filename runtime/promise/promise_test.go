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
