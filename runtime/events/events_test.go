package events

import (
	"testing"

	promise "github.com/nnstd/gun/runtime/promise"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestAsJSValueExportsEventsSurface(t *testing.T) {
	if AsJSValue.Get("EventEmitter").TypeString() != "function" {
		t.Fatal("expected EventEmitter export")
	}
	emitter := AsJSValue.Get("EventEmitter").Call()
	if emitter.Get("on").TypeString() != "function" {
		t.Fatal("expected EventEmitter instance methods")
	}
}

func TestOnceReturnsPromise(t *testing.T) {
	emitter := AsJSValue.Get("EventEmitter").Call()
	p := AsJSValue.Get("once").Call(emitter, jsvalue.NewString("ready"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected events.once to return a Promise")
	}
}
