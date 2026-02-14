package compiler

import (
	"go/ast"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ModuleCallTransformer transforms a method call on a variable whose type
// belongs to a known module. Return nil to fall through to default handling.
type ModuleCallTransformer func(t *Transformer, objNode *sitter.Node, method string, argsNode *sitter.Node) ast.Expr

// moduleCallTransformers maps module package names (matching varTypes values)
// to their call transformers.
var moduleCallTransformers = map[string]ModuleCallTransformer{}

// RegisterCallTransformer registers a call transformer for a module type.
// The modType should match the goPkgName used in varTypes (e.g. "hono").
func RegisterCallTransformer(modType string, fn ModuleCallTransformer) {
	moduleCallTransformers[modType] = fn
}
