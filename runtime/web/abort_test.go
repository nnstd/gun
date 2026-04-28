package web

import (
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

func TestAbortControllerAbortDispatchesOnce(t *testing.T) {
	controller := AbortController.Call()
	signal := controller.Get("signal")

	if signal.Get("aborted").Bool() {
		t.Fatal("signal should start unaborted")
	}
	if signal.Get("reason").TypeString() != "undefined" {
		t.Fatalf("initial reason = %s, want undefined", signal.Get("reason").TypeString())
	}

	count := 0
	signal.MethodCall("addEventListener", jsvalue.NewString("abort"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		count++
		if len(args) == 0 || args[0].Get("type").String() != "abort" {
			t.Fatalf("abort listener event = %v", args)
		}
		return jsvalue.NewUndefined()
	}))
	signal.Set("onabort", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		count++
		return jsvalue.NewUndefined()
	}))

	reason := jsvalue.NewString("stop")
	controller.MethodCall("abort", reason)
	controller.MethodCall("abort", jsvalue.NewString("ignored"))

	if !signal.Get("aborted").Bool() {
		t.Fatal("signal should be aborted")
	}
	if signal.Get("reason") != reason {
		t.Fatalf("reason was not preserved")
	}
	if count != 2 {
		t.Fatalf("abort handlers called %d times, want 2", count)
	}
}

func TestAbortSignalAbortAndThrowIfAborted(t *testing.T) {
	reason := jsvalue.NewString("because")
	signal := AbortSignal.Get("abort").Call(reason)

	if !signal.Get("aborted").Bool() {
		t.Fatal("static abort signal should be aborted")
	}
	if signal.Get("reason") != reason {
		t.Fatal("static abort reason was not preserved")
	}

	defer func() {
		if recovered := recover(); recovered != reason {
			t.Fatalf("throwIfAborted panic = %v, want reason", recovered)
		}
	}()
	signal.MethodCall("throwIfAborted")
}

func TestAbortSignalAnyUsesFirstAbortReason(t *testing.T) {
	a := AbortController.Call()
	b := AbortController.Call()
	combined := AbortSignal.Get("any").Call(jsvalue.NewArray(a.Get("signal"), b.Get("signal")))

	seen := 0
	combined.MethodCall("addEventListener", jsvalue.NewString("abort"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		seen++
		return jsvalue.NewUndefined()
	}))

	reason := jsvalue.NewString("first")
	b.MethodCall("abort", reason)
	a.MethodCall("abort", jsvalue.NewString("second"))

	if !combined.Get("aborted").Bool() {
		t.Fatal("combined signal should abort")
	}
	if combined.Get("reason") != reason {
		t.Fatal("combined signal did not keep first abort reason")
	}
	if seen != 1 {
		t.Fatalf("combined abort events = %d, want 1", seen)
	}
}

func TestAbortSignalTimeout(t *testing.T) {
	signal := AbortSignal.Get("timeout").Call(jsvalue.NewNumber(1))
	if signal.Get("aborted").Bool() {
		t.Fatal("timeout signal should not abort synchronously")
	}

	done := make(chan struct{}, 1)
	signal.MethodCall("addEventListener", jsvalue.NewString("abort"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		done <- struct{}{}
		return jsvalue.NewUndefined()
	}))

	go eventloop.Default.Run()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout signal did not abort")
	}

	if !signal.Get("aborted").Bool() {
		t.Fatal("timeout signal should be aborted")
	}
	if got := signal.Get("reason").Get("name").String(); got != "TimeoutError" {
		t.Fatalf("timeout reason name = %q, want TimeoutError", got)
	}
}

func TestAbortGlobalsRegistered(t *testing.T) {
	globals := jsvalue.Globals()
	if globals["AbortController"] != AbortController {
		t.Fatal("AbortController global not registered")
	}
	if globals["AbortSignal"] != AbortSignal {
		t.Fatal("AbortSignal global not registered")
	}
}
