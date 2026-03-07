package symbol

import "testing"

func TestDefineAndLookup(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("x", KindVariable)
	if sym.ID == 0 {
		t.Fatal("expected non-zero symbol ID")
	}
	if sym.OriginalName != "x" {
		t.Fatalf("expected original name 'x', got %q", sym.OriginalName)
	}
	if sym.Kind != KindVariable {
		t.Fatalf("expected KindVariable, got %d", sym.Kind)
	}

	found := tab.Lookup("x")
	if found != sym {
		t.Fatal("Lookup should return the same symbol as Define")
	}
}

func TestLookupNotFound(t *testing.T) {
	tab := NewTable()
	if tab.Lookup("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent symbol")
	}
}

func TestScopeNesting(t *testing.T) {
	tab := NewTable()
	outer := tab.Define("a", KindVariable)

	tab.PushScope()
	inner := tab.Define("b", KindVariable)

	// Both visible from inner scope
	if tab.Lookup("a") != outer {
		t.Fatal("outer symbol should be visible from inner scope")
	}
	if tab.Lookup("b") != inner {
		t.Fatal("inner symbol should be visible from inner scope")
	}

	tab.PopScope()

	// Only outer visible after pop
	if tab.Lookup("a") != outer {
		t.Fatal("outer symbol should still be visible after pop")
	}
	if tab.Lookup("b") != nil {
		t.Fatal("inner symbol should not be visible after pop")
	}
}

func TestScopeShadowing(t *testing.T) {
	tab := NewTable()
	outer := tab.Define("x", KindVariable)

	tab.PushScope()
	inner := tab.Define("x", KindVariable)

	// Inner scope shadows outer
	if tab.Lookup("x") != inner {
		t.Fatal("inner 'x' should shadow outer 'x'")
	}
	if inner.ID == outer.ID {
		t.Fatal("shadowed symbols should have different IDs")
	}

	tab.PopScope()

	// Outer is restored
	if tab.Lookup("x") != outer {
		t.Fatal("outer 'x' should be restored after pop")
	}
}

func TestLookupLocal(t *testing.T) {
	tab := NewTable()
	tab.Define("a", KindVariable)

	tab.PushScope()
	tab.Define("b", KindVariable)

	// LookupLocal only sees current scope
	if tab.LookupLocal("b") == nil {
		t.Fatal("LookupLocal should find 'b' in current scope")
	}
	if tab.LookupLocal("a") != nil {
		t.Fatal("LookupLocal should not find 'a' from parent scope")
	}
	tab.PopScope()
}

func TestEmitNameBasic(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("foo", KindVariable)

	name := tab.EmitName(sym)
	if name != "foo" {
		t.Fatalf("expected 'foo', got %q", name)
	}
}

func TestEmitNameExported(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("foo", KindFunction)
	sym.Exported = true

	name := tab.EmitName(sym)
	if name != "Foo" {
		t.Fatalf("expected 'Foo', got %q", name)
	}
}

func TestEmitNameCollision(t *testing.T) {
	tab := NewTable()

	// Both "foo" (exported) and "Foo" (variable) would emit as "Foo"
	sym1 := tab.Define("foo", KindFunction)
	sym1.Exported = true

	sym2 := tab.Define("Foo", KindVariable)
	sym2.Exported = true

	name1 := tab.EmitName(sym1)
	name2 := tab.EmitName(sym2)

	if name1 == name2 {
		t.Fatalf("emitted names should differ: got %q and %q", name1, name2)
	}
	if name1 != "Foo" {
		t.Fatalf("first symbol should get 'Foo', got %q", name1)
	}
	// Second should get a suffixed name
	if name2 == "Foo" {
		t.Fatal("second symbol should NOT get 'Foo'")
	}
}

func TestEmitNameGoKeyword(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("string", KindVariable)

	name := tab.EmitName(sym)
	if name != "string_" {
		t.Fatalf("expected 'string_', got %q", name)
	}
}

func TestEmitNameDollarSign(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("$count", KindVariable)

	name := tab.EmitName(sym)
	if name != "_count" {
		t.Fatalf("expected '_count', got %q", name)
	}
}

func TestEmitNameStable(t *testing.T) {
	// Same symbol emitted twice should return the same name
	tab := NewTable()
	sym := tab.Define("x", KindVariable)

	name1 := tab.EmitName(sym)
	name2 := tab.EmitName(sym)
	if name1 != name2 {
		t.Fatalf("expected stable emit: %q vs %q", name1, name2)
	}
}

func TestGetByID(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("x", KindVariable)

	got := tab.Get(sym.ID)
	if got != sym {
		t.Fatal("Get should return the symbol by ID")
	}
	if tab.Get(9999) != nil {
		t.Fatal("Get should return nil for unknown ID")
	}
}

func TestIsGlobalScope(t *testing.T) {
	tab := NewTable()
	if !tab.IsGlobalScope() {
		t.Fatal("should start in global scope")
	}
	tab.PushScope()
	if tab.IsGlobalScope() {
		t.Fatal("should not be global scope after push")
	}
	tab.PopScope()
	if !tab.IsGlobalScope() {
		t.Fatal("should be global scope after pop")
	}
}

func TestScopeDepth(t *testing.T) {
	tab := NewTable()
	if tab.ScopeDepth() != 0 {
		t.Fatalf("expected depth 0, got %d", tab.ScopeDepth())
	}
	tab.PushScope()
	if tab.ScopeDepth() != 1 {
		t.Fatalf("expected depth 1, got %d", tab.ScopeDepth())
	}
	tab.PushScope()
	if tab.ScopeDepth() != 2 {
		t.Fatalf("expected depth 2, got %d", tab.ScopeDepth())
	}
	tab.PopScope()
	tab.PopScope()
	if tab.ScopeDepth() != 0 {
		t.Fatalf("expected depth 0, got %d", tab.ScopeDepth())
	}
}

func TestPopGlobalScopePanics(t *testing.T) {
	tab := NewTable()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when popping global scope")
		}
	}()
	tab.PopScope()
}

func TestReserveName(t *testing.T) {
	tab := NewTable()

	// Reserve "Default" for a cross-file export
	crossFile := tab.Define("default", KindVariable)
	crossFile.Exported = true
	tab.ReserveName("Default", crossFile)

	// Now a local "default" export should get a suffixed name
	local := tab.Define("default2", KindVariable)
	local.Exported = true
	// Manually set original name to produce "Default" collision
	local.OriginalName = "default"
	name := tab.EmitName(local)
	if name == "Default" {
		t.Fatal("local should not get 'Default' since it was reserved")
	}
}

func TestUniqueIDs(t *testing.T) {
	tab := NewTable()
	ids := make(map[ID]bool)
	for i := 0; i < 100; i++ {
		sym := tab.Define("v", KindVariable)
		if ids[sym.ID] {
			t.Fatalf("duplicate ID: %d", sym.ID)
		}
		ids[sym.ID] = true
	}
}

func TestForEach(t *testing.T) {
	tab := NewTable()
	tab.Define("a", KindVariable)
	tab.Define("b", KindFunction)
	tab.PushScope()
	tab.Define("c", KindParameter)
	tab.PopScope()

	count := 0
	tab.ForEach(func(sym *Symbol) {
		count++
	})
	if count != 3 {
		t.Fatalf("expected 3 symbols, got %d", count)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"foo", "foo"},
		{"$count", "_count"},
		{"string", "string_"},
		{"map", "map_"},
		{"nil", "nil_"},
		{"normal", "normal"},
		{"a$b$c", "a_b_c"},
	}
	for _, tt := range tests {
		got := Sanitize(tt.input)
		if got != tt.want {
			t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"foo", "Foo"},
		{"Foo", "Foo"},
		{"", ""},
		{"a", "A"},
		{"myFunc", "MyFunc"},
	}
	for _, tt := range tests {
		got := Capitalize(tt.input)
		if got != tt.want {
			t.Errorf("Capitalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultIsJSValue(t *testing.T) {
	tab := NewTable()
	sym := tab.Define("x", KindVariable)
	if !sym.IsJSValue {
		t.Fatal("new symbols should default to IsJSValue=true")
	}
}
