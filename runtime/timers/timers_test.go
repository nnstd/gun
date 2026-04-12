package timers

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestPromisesAsJSValueExportsTimeout(t *testing.T) {
	if PromisesAsJSValue.Get("setTimeout").TypeString() != "function" {
		t.Fatal("expected timers/promises setTimeout export")
	}
	p := PromisesAsJSValue.Get("setTimeout").Call(jsvalue.NewNumber(0))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected setTimeout() to return a Promise")
	}
}
