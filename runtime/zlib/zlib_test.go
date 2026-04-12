package zlib

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestPromisesAsJSValueExportsGzip(t *testing.T) {
	if PromisesAsJSValue.Get("gzip").TypeString() != "function" {
		t.Fatal("expected zlib/promises gzip export")
	}
	p := PromisesAsJSValue.Get("gzip").Call(jsvalue.NewString("hello"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected gzip() to return a Promise")
	}
}
