package events

import "testing"

func TestAsJSValueExportsEventsSurface(t *testing.T) {
	if AsJSValue.Get("EventEmitter").TypeString() != "function" {
		t.Fatal("expected EventEmitter export")
	}
	emitter := AsJSValue.Get("EventEmitter").Call()
	if emitter.Get("on").TypeString() != "function" {
		t.Fatal("expected EventEmitter instance methods")
	}
}
