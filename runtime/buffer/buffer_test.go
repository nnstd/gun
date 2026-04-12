package buffer

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestAsJSValueExportsBufferSurface(t *testing.T) {
	if AsJSValue.Get("Buffer").TypeString() != "function" {
		t.Fatal("expected Buffer export")
	}
	buf := AsJSValue.Get("Buffer").MethodCall("from", jsvalue.NewString("hi"))
	if got := buf.MethodCall("toString").String(); got != "hi" {
		t.Fatalf("buffer toString = %q, want hi", got)
	}
	if !AsJSValue.Get("Buffer").MethodCall("isBuffer", buf).Bool() {
		t.Fatal("expected isBuffer(buf) to be true")
	}
	if AsJSValue.Get("atob").TypeString() != "function" || AsJSValue.Get("btoa").TypeString() != "function" {
		t.Fatal("expected atob/btoa exports")
	}
}
