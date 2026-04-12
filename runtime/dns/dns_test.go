package dns

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestPromisesAsJSValueExportsLookup(t *testing.T) {
	if PromisesAsJSValue.Get("lookup").TypeString() != "function" {
		t.Fatal("expected dns/promises lookup export")
	}
	p := PromisesAsJSValue.Get("lookup").Call(jsvalue.NewString("localhost"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected lookup() to return a Promise")
	}
}
