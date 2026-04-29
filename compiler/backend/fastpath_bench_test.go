package backend

import (
	"testing"

	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

var lowerBenchSink string

func BenchmarkFastPathComputedPropertyKeyLowering(b *testing.B) {
	symtab := symbol.NewTable()
	obj := symtab.Define("obj", symbol.KindVariable)
	key := symtab.Define("key", symbol.KindVariable)
	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.VarDecl{Kind: hir.VarLet, Declarators: []*hir.Declarator{
				{Symbol: obj, Init: &hir.ObjectLiteral{}},
				{Symbol: key, Init: &hir.Literal{Kind: hir.LitString, Value: "name"}},
			}},
			&hir.FuncDecl{
				Symbol: func() *symbol.Symbol {
					s := symtab.Define("read", symbol.KindFunction)
					s.FuncInfo = &symbol.FuncInfo{}
					return s
				}(),
				Body: &hir.BlockStmt{Stmts: []hir.Stmt{
					&hir.ReturnStmt{Value: &hir.ComputedMemberExpr{
						Object:   &hir.Identifier{Sym: obj, Name: "obj"},
						Property: &hir.Identifier{Sym: key, Name: "key"},
					}},
				}},
			},
		},
	}
	b.ResetTimer()
	for b.Loop() {
		file := Lower(mod, context.New(), "", false, context.O0)
		out, err := Generate(file)
		if err != nil {
			b.Fatal(err)
		}
		lowerBenchSink = string(out)
	}
}
