package json

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func expectTypeErrorMessage(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic: %s", want)
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("expected *JSValue panic, got %T", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("panic name = %q, want TypeError", got)
		}
		if got := err.Get("message").String(); got != want {
			t.Fatalf("panic message = %q, want %q", got, want)
		}
	}()
	fn()
}

func TestBoxedPrimitiveJSONParity(t *testing.T) {
	if got := Stringify(jsvalue.Object.Call(jsvalue.NewBool(true))).String(); got != "true" {
		t.Fatalf("JSON.stringify(Object(true)) = %q", got)
	}
	if got := Stringify(jsvalue.Object.Call(jsvalue.NewNumber(42))).String(); got != "42" {
		t.Fatalf("JSON.stringify(Object(42)) = %q", got)
	}
	if got := Stringify(jsvalue.Object.Call(jsvalue.NewString("ab"))).String(); got != "\"ab\"" {
		t.Fatalf("JSON.stringify(Object('ab')) = %q", got)
	}
	expectTypeErrorMessage(t, "JSON.stringify cannot serialize BigInt.", func() {
		_ = Stringify(jsvalue.Object.Call(jsvalue.NewBigInt(1)))
	})
	if got := Stringify(jsvalue.Object.Call(jsvalue.NewSymbol("x"))).String(); got != "{}" {
		t.Fatalf("JSON.stringify(Object(Symbol())) = %q", got)
	}
}
