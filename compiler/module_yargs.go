package compiler

import (
	"go/ast"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func init() {
	RegisterCallTransformer("yargs", transformYargsCall)
}

// transformYargsCall handles method calls on yargs instances.
// Rewrites .command(pattern, desc, builder, handler) with typed callback params.
func transformYargsCall(t *Transformer, objNode *sitter.Node, method string, argsNode *sitter.Node) ast.Expr {
	if method != "command" {
		return nil
	}

	obj := t.transformExpr(objNode)
	fun := selectorExpr(obj, capitalize(method))

	var args []ast.Expr
	if argsNode != nil {
		for i := uint(0); i < argsNode.NamedChildCount(); i++ {
			child := argsNode.NamedChild(i)
			if i == 2 && isCallbackNode(child) {
				args = append(args, transformYargsBuilder(t, child))
			} else if i == 3 && isCallbackNode(child) {
				args = append(args, transformYargsHandler(t, child))
			} else {
				if e := t.transformExpr(child); e != nil {
					args = append(args, e)
				}
			}
		}
	}

	return callExpr(fun, args...)
}

func isCallbackNode(node *sitter.Node) bool {
	switch node.Kind() {
	case "arrow_function", "function_expression", "function":
		return true
	}
	return false
}

// transformYargsBuilder transforms a builder callback, rewriting parameter types
// to *yargs.Yargs and return type to *yargs.Yargs.
func transformYargsBuilder(t *Transformer, node *sitter.Node) ast.Expr {
	t.addImport("github.com/nnstd/gun/runtime/yargs")

	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	paramNode := node.ChildByFieldName("parameter")

	yargsType := ptrType(selectorExpr(ident("yargs"), "Yargs"))

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
				fields = append(fields, field(pName, yargsType))
			}
		}
		params = fieldList(fields...)
	} else if paramNode != nil {
		pName := paramNode.Utf8Text(t.source)
		params = fieldList(field(pName, yargsType))
	} else {
		params = fieldList()
	}

	results := fieldList(field("", yargsType))

	// Push parameter names so they shadow imports inside the body
	paramNames := extractParamNames(paramsNode, t.source)
	if paramNode != nil {
		paramNames = []string{paramNode.Utf8Text(t.source)}
	}
	t.pushScope(paramNames)
	defer t.popScope()

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

// transformYargsHandler transforms a handler callback, rewriting parameter types
// to *yargs.Argv.
func transformYargsHandler(t *Transformer, node *sitter.Node) ast.Expr {
	t.addImport("github.com/nnstd/gun/runtime/yargs")

	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	paramNode := node.ChildByFieldName("parameter")

	argvType := ptrType(selectorExpr(ident("yargs"), "Argv"))

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
				fields = append(fields, field(pName, argvType))
			}
		}
		params = fieldList(fields...)
	} else if paramNode != nil {
		pName := paramNode.Utf8Text(t.source)
		params = fieldList(field(pName, argvType))
	} else {
		params = fieldList()
	}

	// Push parameter names so they shadow imports inside the body
	paramNames := extractParamNames(paramsNode, t.source)
	if paramNode != nil {
		paramNames = []string{paramNode.Utf8Text(t.source)}
	}
	t.pushScope(paramNames)
	defer t.popScope()

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
		Type: &ast.FuncType{Params: params},
		Body: body,
	}
}
