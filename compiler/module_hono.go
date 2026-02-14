package compiler

import (
	"go/ast"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func init() {
	RegisterCallTransformer("hono", transformHonoCall)
}

// transformHonoCall handles method calls on Hono app instances.
// Rewrites app.get("/", handler) → app.Get("/", handler) with typed handler params.
func transformHonoCall(t *Transformer, objNode *sitter.Node, method string, argsNode *sitter.Node) ast.Expr {
	if !isHTTPMethod(method) {
		return nil
	}

	obj := t.transformExpr(objNode)
	fun := selectorExpr(obj, capitalize(method))

	var args []ast.Expr
	if argsNode != nil {
		for i := uint(0); i < argsNode.NamedChildCount(); i++ {
			child := argsNode.NamedChild(i)
			switch child.Kind() {
			case "arrow_function", "function_expression", "function":
				args = append(args, transformHonoHandler(t, child))
			default:
				if e := t.transformExpr(child); e != nil {
					args = append(args, e)
				}
			}
		}
	}

	return callExpr(fun, args...)
}

func isHTTPMethod(name string) bool {
	switch name {
	case "get", "post", "put", "delete", "patch":
		return true
	}
	return false
}

// transformHonoHandler transforms a route handler, rewriting parameter types
// from `any` to `*hono.Context`.
func transformHonoHandler(t *Transformer, node *sitter.Node) ast.Expr {
	t.addImport("gun/runtime/hono")

	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	paramNode := node.ChildByFieldName("parameter")

	var params *ast.FieldList
	if paramsNode != nil {
		var fields []*ast.Field
		for i := uint(0); i < paramsNode.NamedChildCount(); i++ {
			param := paramsNode.NamedChild(i)
			if param.Kind() == "required_parameter" || param.Kind() == "optional_parameter" {
				nameNode := param.ChildByFieldName("pattern")
				pName := "_"
				if nameNode != nil {
					pName = nameNode.Utf8Text(t.source)
				}
				fields = append(fields, field(pName, ptrType(selectorExpr(ident("hono"), "Context"))))
			}
		}
		params = fieldList(fields...)
	} else if paramNode != nil {
		pName := paramNode.Utf8Text(t.source)
		params = fieldList(field(pName, ptrType(selectorExpr(ident("hono"), "Context"))))
	} else {
		params = fieldList()
	}

	results := fieldList(field("", ident("any")))

	var body *ast.BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = t.transformBlock(bodyNode)
		} else {
			if expr := t.transformExpr(bodyNode); expr != nil {
				body = blockStmt(returnStmt(expr))
			} else {
				body = blockStmt()
			}
		}
	} else {
		body = blockStmt()
	}

	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}
