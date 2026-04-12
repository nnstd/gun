package json

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestAsJSValueExportsJSONMethods(t *testing.T) {
	if AsJSValue.Get("stringify").TypeString() != "function" {
		t.Fatal("expected stringify export on JSON.AsJSValue")
	}
	if AsJSValue.Get("parse").TypeString() != "function" {
		t.Fatal("expected parse export on JSON.AsJSValue")
	}
	got := AsJSValue.Get("parse").Call(jsvalue.NewString(`"hello"`))
	if got.String() != "hello" {
		t.Fatalf("parse result = %q, want hello", got.String())
	}
}
