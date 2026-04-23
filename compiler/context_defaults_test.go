package compiler

import (
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
