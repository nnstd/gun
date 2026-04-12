package context

import (
	"go/ast"
	"testing"
)

func TestNewContext(t *testing.T) {
	ctx := New()
	if ctx == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRegisterAndLookupGlobal(t *testing.T) {
	ctx := New()
	ctx.RegisterGlobal(&GlobalObject{
		Name: "console",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp Imports) ast.Expr {
			return nil
		},
	})

	g := ctx.LookupGlobal("console")
	if g == nil {
		t.Fatal("expected to find 'console'")
	}
	if g.Name != "console" {
		t.Fatalf("expected name 'console', got %q", g.Name)
	}
	if ctx.LookupGlobal("Math") != nil {
		t.Fatal("should not find unregistered global")
	}
}

func TestRegisterAndLookupGlobalFunc(t *testing.T) {
	ctx := New()
	ctx.RegisterGlobalFunc(&GlobalFunction{
		Name: "parseInt",
		Transform: func(args []ast.Expr, imp Imports) ast.Expr {
			return nil
		},
	})

	fn := ctx.LookupGlobalFunc("parseInt")
	if fn == nil {
		t.Fatal("expected to find 'parseInt'")
	}
	if ctx.LookupGlobalFunc("isNaN") != nil {
		t.Fatal("should not find unregistered global func")
	}
}

func TestRegisterAndLookupConstructor(t *testing.T) {
	ctx := New()
	ctx.RegisterConstructor(&Constructor{
		Name: "Map",
		Transform: func(args []ast.Expr, imp Imports) ast.Expr {
			return nil
		},
	})

	ctor := ctx.LookupConstructor("Map")
	if ctor == nil {
		t.Fatal("expected to find 'Map'")
	}
	if ctx.LookupConstructor("Set") != nil {
		t.Fatal("should not find unregistered constructor")
	}
}

func TestRegisterAndLookupIdentifier(t *testing.T) {
	ctx := New()
	ctx.RegisterIdentifier(&IdentifierMapping{
		Name: "undefined",
		Transform: func(imp Imports) ast.Expr {
			return &ast.Ident{Name: "nil"}
		},
	})

	id := ctx.LookupIdentifier("undefined")
	if id == nil {
		t.Fatal("expected to find 'undefined'")
	}
	if ctx.LookupIdentifier("null") != nil {
		t.Fatal("should not find unregistered identifier")
	}
}

func TestRegisterAndLookupModule(t *testing.T) {
	ctx := New()
	ctx.RegisterModule("fs", &ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/fs",
		GoPkgName:    "fs",
		SymbolOverrides: map[string]SymbolOverride{
			"readFile": {GoSymbol: "ReadFileSync"},
		},
	})

	mod := ctx.LookupModule("fs")
	if mod == nil {
		t.Fatal("expected to find 'fs'")
	}
	if mod.GoImportPath != "github.com/nnstd/gun/runtime/fs" {
		t.Fatalf("unexpected import path: %q", mod.GoImportPath)
	}
	override, ok := mod.SymbolOverrides["readFile"]
	if !ok {
		t.Fatal("expected symbol override for 'readFile'")
	}
	if override.GoSymbol != "ReadFileSync" {
		t.Fatalf("unexpected go symbol: %q", override.GoSymbol)
	}
	if ctx.LookupModule("path") != nil {
		t.Fatal("should not find unregistered module")
	}
}

func TestIsKnownGlobal(t *testing.T) {
	ctx := New()

	// Registering a global object marks it as known
	ctx.RegisterGlobal(&GlobalObject{Name: "console"})
	if !ctx.IsKnownGlobal("console") {
		t.Fatal("console should be a known global")
	}

	// RegisterIdentifier does NOT mark as known global (by design).
	// Use MarkKnownGlobal separately for identifiers that need it.
	ctx.RegisterIdentifier(&IdentifierMapping{
		Name:      "undefined",
		Transform: func(imp Imports) ast.Expr { return nil },
	})
	ctx.MarkKnownGlobal("undefined")
	if !ctx.IsKnownGlobal("undefined") {
		t.Fatal("undefined should be a known global after MarkKnownGlobal")
	}

	// MarkKnownGlobal adds to the set
	ctx.MarkKnownGlobal("globalThis")
	if !ctx.IsKnownGlobal("globalThis") {
		t.Fatal("globalThis should be a known global after MarkKnownGlobal")
	}

	// Unregistered names are not known
	if ctx.IsKnownGlobal("foo") {
		t.Fatal("foo should not be a known global")
	}
}

// mockImports implements Imports for testing.
type mockImports struct {
	imports       []string
	aliasImports  []string
}

func (m *mockImports) AddImport(path string) {
	m.imports = append(m.imports, path)
}

func (m *mockImports) AddAliasedImport(path, alias string) {
	m.aliasImports = append(m.aliasImports, path+"="+alias)
}

func TestTransformBuiltinCall(t *testing.T) {
	ctx := New()
	called := false
	ctx.RegisterGlobal(&GlobalObject{
		Name: "Math",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp Imports) ast.Expr {
			called = true
			if method != "floor" {
				t.Fatalf("expected method 'floor', got %q", method)
			}
			return &ast.Ident{Name: "result"}
		},
	})

	imp := &mockImports{}
	result := ctx.TransformBuiltinCall("Math", "floor", nil, false, imp)
	if !called {
		t.Fatal("transform was not called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Unknown object returns nil
	result = ctx.TransformBuiltinCall("Unknown", "method", nil, false, imp)
	if result != nil {
		t.Fatal("expected nil for unknown object")
	}
}

func TestTransformGlobalCall(t *testing.T) {
	ctx := New()
	ctx.RegisterGlobalFunc(&GlobalFunction{
		Name: "parseInt",
		Transform: func(args []ast.Expr, imp Imports) ast.Expr {
			return &ast.Ident{Name: "parsed"}
		},
	})

	imp := &mockImports{}
	result := ctx.TransformGlobalCall("parseInt", nil, imp)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	result = ctx.TransformGlobalCall("unknown", nil, imp)
	if result != nil {
		t.Fatal("expected nil for unknown function")
	}
}

func TestTransformBuiltinNew(t *testing.T) {
	ctx := New()
	ctx.RegisterConstructor(&Constructor{
		Name: "Map",
		Transform: func(args []ast.Expr, imp Imports) ast.Expr {
			return &ast.Ident{Name: "newMap"}
		},
	})

	imp := &mockImports{}
	result := ctx.TransformBuiltinNew("Map", nil, imp)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	result = ctx.TransformBuiltinNew("Unknown", nil, imp)
	if result != nil {
		t.Fatal("expected nil for unknown constructor")
	}
}

func TestTransformIdentifier(t *testing.T) {
	ctx := New()
	ctx.RegisterIdentifier(&IdentifierMapping{
		Name: "undefined",
		Transform: func(imp Imports) ast.Expr {
			return &ast.Ident{Name: "nil"}
		},
	})

	imp := &mockImports{}
	result := ctx.TransformIdentifier("undefined", imp)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	ident, ok := result.(*ast.Ident)
	if !ok || ident.Name != "nil" {
		t.Fatal("expected nil ident")
	}

	result = ctx.TransformIdentifier("unknown", imp)
	if result != nil {
		t.Fatal("expected nil for unknown identifier")
	}
}

func TestTransformBuiltinMember(t *testing.T) {
	ctx := New()
	ctx.RegisterGlobal(&GlobalObject{
		Name: "process",
		TransformMember: func(prop string, imp Imports) ast.Expr {
			if prop == "env" {
				return &ast.Ident{Name: "processEnv"}
			}
			return nil
		},
	})

	imp := &mockImports{}
	result := ctx.TransformBuiltinMember("process", "env", imp)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	result = ctx.TransformBuiltinMember("process", "unknown", imp)
	if result != nil {
		t.Fatal("expected nil for unknown property")
	}

	result = ctx.TransformBuiltinMember("unknown", "env", imp)
	if result != nil {
		t.Fatal("expected nil for unknown object")
	}
}
