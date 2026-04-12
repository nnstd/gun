package stream

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestPromisesAsJSValueExportsPromiseHelpers(t *testing.T) {
	if PromisesAsJSValue.Get("pipeline").TypeString() != "function" {
		t.Fatal("expected stream/promises pipeline export")
	}
	r := Readable.Call()
	w := Writable.Call()
	p := PromisesAsJSValue.Get("pipeline").Call(r, w)
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected pipeline() to return a Promise")
	}
}
