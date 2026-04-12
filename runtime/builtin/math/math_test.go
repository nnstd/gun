package math

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestAsJSValueExportsMathMethods(t *testing.T) {
	if got := AsJSValue.Get("floor").Call(jsvalue.NewNumber(1.9)).Number(); got != 1 {
		t.Fatalf("floor = %v, want 1", got)
	}
	if got := AsJSValue.Get("min").Call(jsvalue.NewNumber(4), jsvalue.NewNumber(2)).Number(); got != 2 {
		t.Fatalf("min = %v, want 2", got)
	}
	if AsJSValue.Get("random").TypeString() != "function" {
		t.Fatal("expected random export on Math.AsJSValue")
	}
}
