package jsvalue

import "testing"

func TestFromMap(t *testing.T) {
	m := map[string]*JSValue{
		"name": NewString("go"),
		"ver":  NewInt(1),
	}
	v := From(m)
	if v.typ != TypeObject {
		t.Fatalf("expected TypeObject, got %v", v.typ)
	}
	if got := v.Get("name").String(); got != "go" {
		t.Errorf("name: got %q, want %q", got, "go")
	}
	if got := int(v.Get("ver").Number()); got != 1 {
		t.Errorf("ver: got %d, want 1", got)
	}
}

func TestFromPassthrough(t *testing.T) {
	orig := NewString("hello")
	v := From(orig)
	if v != orig {
		t.Error("From(*JSValue) should return the same pointer")
	}
}

func TestFromPrimitives(t *testing.T) {
	if v := From(42); v.typ != TypeNumber || int(v.Number()) != 42 {
		t.Errorf("From(int): got type=%v num=%v", v.typ, v.Number())
	}
	if v := From(3.14); v.typ != TypeNumber || v.Number() != 3.14 {
		t.Errorf("From(float64): got type=%v num=%v", v.typ, v.Number())
	}
	if v := From(true); v.typ != TypeBoolean || !v.Bool() {
		t.Errorf("From(bool): got type=%v bool=%v", v.typ, v.Bool())
	}
	if v := From("hi"); v.typ != TypeString || v.String() != "hi" {
		t.Errorf("From(string): got type=%v str=%v", v.typ, v.String())
	}
	if v := From(nil); v.typ != TypeNull {
		t.Errorf("From(nil): got type=%v, want TypeNull", v.typ)
	}
}
