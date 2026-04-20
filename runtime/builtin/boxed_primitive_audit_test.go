package jsvalue

import "testing"

func expectTypeErrorMessage(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic: %s", want)
		}
		err, ok := r.(*JSValue)
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

func TestBoxedPrimitiveAuditBigIntAndSymbol(t *testing.T) {
	big1 := Object.Call(NewBigInt(1))
	big2 := Object.Call(NewBigInt(1))
	if big1 == big2 {
		t.Fatal("Object(1n) should return a fresh wrapper each time")
	}
	if got := big1.MethodCall("valueOf").BigInt(); got != 1 {
		t.Fatalf("Object(1n).valueOf() = %d, want 1", got)
	}
	if big1.Get("constructor") != BigIntCtor {
		t.Fatal("Object(1n).constructor must be BigInt")
	}
	if got := ObjectPrototype.Get("toString").MethodCall("call", big1).String(); got != "[object BigInt]" {
		t.Fatalf("Object.prototype.toString.call(Object(1n)) = %q", got)
	}

	sym := NewSymbol("x")
	symBox1 := Object.Call(sym)
	symBox2 := Object.Call(sym)
	if symBox1 == symBox2 {
		t.Fatal("Object(Symbol('x')) should return a fresh wrapper each time")
	}
	if got := symBox1.MethodCall("valueOf"); got != sym {
		t.Fatal("Object(Symbol('x')).valueOf() must return original symbol")
	}
	if symBox1.Get("constructor") != Symbol_ {
		t.Fatal("Object(Symbol('x')).constructor must be Symbol")
	}
	if got := ObjectPrototype.Get("toString").MethodCall("call", symBox1).String(); got != "[object Symbol]" {
		t.Fatalf("Object.prototype.toString.call(Object(Symbol)) = %q", got)
	}
}

func TestBoxedPrimitiveOwnNamesAndAssign(t *testing.T) {
	if got := len(Object.Get("getOwnPropertyNames").Call(NewBigInt(1)).Array()); got != 0 {
		t.Fatalf("Object.getOwnPropertyNames(1n) len = %d, want 0", got)
	}
	if got := len(Object.Get("getOwnPropertyNames").Call(NewSymbol("x")).Array()); got != 0 {
		t.Fatalf("Object.getOwnPropertyNames(Symbol()) len = %d, want 0", got)
	}
	strNames := Object.Get("getOwnPropertyNames").Call(NewString("ab")).Array()
	if len(strNames) != 3 || strNames[0].String() != "0" || strNames[1].String() != "1" || strNames[2].String() != "length" {
		t.Fatalf("Object.getOwnPropertyNames('ab') mismatch: %#v", strNames)
	}

	if got := Assign(NewBigInt(1), ObjectFrom("a", NewNumber(1))).String(); got != "1" {
		t.Fatalf("String(Object.assign(1n, {a:1})) = %q, want 1", got)
	}
	expectTypeErrorMessage(t, "Cannot convert a symbol to a string", func() {
		_ = Assign(NewSymbol("x"), ObjectFrom("a", NewNumber(1))).String()
	})
}

func TestPrimitiveObjectSurfaceParity(t *testing.T) {
	if got := Object.Get("setPrototypeOf").Call(NewBigInt(1), NewObject()).TypeString(); got != "bigint" {
		t.Fatalf("typeof Object.setPrototypeOf(1n,{}) = %q, want bigint", got)
	}
	if got := Object.Get("setPrototypeOf").Call(NewSymbol("x"), NewObject()).TypeString(); got != "symbol" {
		t.Fatalf("typeof Object.setPrototypeOf(Symbol(),{}) = %q, want symbol", got)
	}
	expectTypeErrorMessage(t, "Properties can only be defined on Objects.", func() {
		DefineProperty(NewBigInt(1), NewString("x"), ObjectFrom("value", NewNumber(1)))
	})
	expectTypeErrorMessage(t, "Properties can only be defined on Objects.", func() {
		DefineProperty(NewSymbol("x"), NewString("x"), ObjectFrom("value", NewNumber(1)))
	})
	expectTypeErrorMessage(t, "Reflect.set requires the first argument be an object", func() {
		Reflect.Get("set").Call(NewBigInt(1), NewString("x"), NewNumber(1))
	})
	expectTypeErrorMessage(t, "Reflect.set requires the first argument be an object", func() {
		Reflect.Get("set").Call(NewSymbol("x"), NewString("x"), NewNumber(1))
	})
	if got := Object.Get("getPrototypeOf").Call(NewBigInt(1)); got != BigIntPrototype {
		t.Fatal("Object.getPrototypeOf(1n) must be BigInt.prototype")
	}
	if got := Object.Get("getPrototypeOf").Call(NewSymbol("x")); got != SymbolPrototype {
		t.Fatal("Object.getPrototypeOf(Symbol()) must be Symbol.prototype")
	}
}
