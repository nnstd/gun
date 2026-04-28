package compiler

import (
	"go/ast"
	"testing"

	tcontext "github.com/nnstd/gun/compiler/context"
)

func TestRegisterDefaultBuiltinsIncludesDgram(t *testing.T) {
	ctx := tcontext.New()
	RegisterDefaultBuiltins(ctx)

	mod := ctx.LookupModule("dgram")
	if mod == nil {
		t.Fatal("expected dgram module mapping")
	}
	if mod.GoImportPath != "github.com/nnstd/gun/runtime/dgram" {
		t.Fatalf("unexpected dgram import path: %q", mod.GoImportPath)
	}
	if !mod.UseAsJSValue {
		t.Fatal("expected dgram to use AsJSValue")
	}
}

type testImports struct{}

func (testImports) AddImport(path string)               {}
func (testImports) AddAliasedImport(path, alias string) {}

func TestRegisterDefaultBuiltinsIncludesFetch(t *testing.T) {
	ctx := tcontext.New()
	RegisterDefaultBuiltins(ctx)

	if !ctx.IsKnownGlobal("fetch") {
		t.Fatal("expected fetch to be a known global")
	}
	if ctx.LookupIdentifier("fetch") == nil {
		t.Fatal("expected fetch identifier mapping")
	}
	if expr := ctx.TransformGlobalCall("fetch", []ast.Expr{}, testImports{}); expr == nil {
		t.Fatal("expected fetch global call transform")
	}
}
