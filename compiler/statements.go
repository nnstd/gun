package compiler

import (
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// isNumericType returns true if the type name represents a numeric Go type.
func isNumericType(typeName string) bool {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return true
	}
	return false
}

func (t *Transformer) transformStmt(node *sitter.Node) ast.Stmt {
	if node == nil {
		return nil
	}

	switch node.Kind() {
	case "expression_statement":
		if node.NamedChildCount() > 0 {
			child := node.NamedChild(0)
			// Skip "use strict" directives
			if child.Kind() == "string" || child.Kind() == "string_literal" {
				return nil
			}
			// Assignment expressions should become assignment statements
			if child.Kind() == "assignment_expression" {
				leftNode := child.ChildByFieldName("left")
				rightNode := child.ChildByFieldName("right")
				if leftNode != nil && rightNode != nil {
					// module.exports = expr → treat as export default (handled at top level)
					if leftNode.Kind() == "member_expression" && leftNode.Utf8Text(t.source) == "module.exports" {
						return nil
					}
					// Skip member assignments on package-level function vars.
					// JS functions are objects and can have properties; Go functions cannot.
					if leftNode.Kind() == "member_expression" {
						objNode := leftNode.ChildByFieldName("object")
						if objNode != nil && objNode.Kind() == "identifier" {
							if t.funcVarNames[objNode.Utf8Text(t.source)] {
								return nil
							}
						}
					}
					// mapLocal[x][y] = value or mapLocal.x[y] = value → (...).Set(key, jsvalue.From(value))
					// The first access on a map local returns *jsvalue.JSValue.
					if leftNode.Kind() == "subscript_expression" {
						subObj := leftNode.ChildByFieldName("object")
						if subObj != nil && (subObj.Kind() == "subscript_expression" || subObj.Kind() == "member_expression") {
							innerObj := subObj.ChildByFieldName("object")
							if innerObj != nil && innerObj.Kind() == "identifier" && t.mapLocals[innerObj.Utf8Text(t.source)] {
								inner := t.transformExpr(subObj)
								outerIdx := leftNode.ChildByFieldName("index")
								outerIdxExpr := t.transformExpr(outerIdx)
								rhs := t.transformExpr(rightNode)
								if inner != nil && outerIdxExpr != nil && rhs != nil {
									if outerIdx.Kind() != "string" && outerIdx.Kind() != "string_literal" {
										t.addImport("fmt")
										outerIdxExpr = callExpr(selectorExpr(ident("fmt"), "Sprint"), outerIdxExpr)
									}
									rhs = t.wrapAsJSValue(rhs)
									return exprStmt(callExpr(selectorExpr(inner, "Set"), outerIdxExpr, rhs))
								}
							}
						}
					}
					// obj[key] = value on JSValue → obj.Set(key, wrappedValue)
					if leftNode.Kind() == "subscript_expression" {
						subObj := leftNode.ChildByFieldName("object")
						subIdx := leftNode.ChildByFieldName("index")
						if subObj != nil && subIdx != nil &&
							subObj.Kind() == "identifier" && t.isUntypedLocal(subObj.Utf8Text(t.source)) {
							obj := t.transformExpr(subObj)
							key := t.transformExpr(subIdx)
							rhs := t.transformExpr(rightNode)
							if obj != nil && key != nil && rhs != nil {
								// Coerce non-string indices to string for .Set()
								if subIdx.Kind() != "string" && subIdx.Kind() != "string_literal" {
									t.addImport("fmt")
									key = callExpr(selectorExpr(ident("fmt"), "Sprint"), key)
								}
								rhs = t.wrapAsJSValue(rhs)
								return exprStmt(callExpr(selectorExpr(obj, "Set"), key, rhs))
							}
						}
					}
					lhs := t.transformExpr(leftNode)
					rhs := t.transformExpr(rightNode)
					if lhs != nil && rhs != nil {
						// Wrap RHS with jsvalue.From() when assigning to an untyped local,
						// but keep nil as-is so pointer nil checks work.
						if leftNode.Kind() == "identifier" && t.isUntypedLocal(leftNode.Utf8Text(t.source)) && !isNilIdent(rhs) {
							rhs = t.wrapAsJSValue(rhs)
						}
						// Wrap RHS when assigning to a JSValue slice element (e.g. args[i] = "").
						if leftNode.Kind() == "subscript_expression" && t.nodeReturnsJSValue(leftNode) && !isNilIdent(rhs) {
							rhs = t.wrapAsJSValue(rhs)
						}
						// When assigning a JSValue-returning expression to a typed local
						// (e.g. i = eatArray(...) where i is int), coerce the return value.
						// For JSValue slice locals, convert via .Array() instead.
						if leftNode.Kind() == "identifier" && t.isTypedLocal(leftNode.Utf8Text(t.source)) && t.nodeReturnsJSValue(rightNode) {
							lname := leftNode.Utf8Text(t.source)
							if t.jsvalueSliceLocals[lname] {
								rhs = callExpr(selectorExpr(rhs, "Array"))
							} else {
								rhs = callExpr(ident("int"), callExpr(selectorExpr(rhs, "Number")))
							}
						}
						// Ternary/conditional assigned to a []*jsvalue.JSValue local
						// generates an IIFE returning `any`. Wrap with jsvalue.ToSlice().
						if leftNode.Kind() == "identifier" {
							lname := leftNode.Utf8Text(t.source)
							if t.jsvalueSliceLocals[lname] {
								rk := rightNode.Kind()
								if rk == "ternary_expression" || rk == "parenthesized_expression" {
									t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
									rhs = callExpr(selectorExpr(ident("jsvalue"), "ToSlice"), rhs)
								}
							}
						}
						return assignStmt([]ast.Expr{lhs}, []ast.Expr{rhs})
					}
				}
			}
			if child.Kind() == "augmented_assignment_expression" {
				leftNode := child.ChildByFieldName("left")
				rightNode := child.ChildByFieldName("right")
				opNode := child.ChildByFieldName("operator")
				if leftNode != nil && rightNode != nil {
					lhs := t.transformExpr(leftNode)
					rhs := t.transformExpr(rightNode)
					opText := ""
					if opNode != nil {
						opText = opNode.Utf8Text(t.source)
					}
					if lhs != nil && rhs != nil {
						// When a typed local (string/int) is combined with a JSValue,
						// coerce the RHS appropriately based on the type.
						if leftNode.Kind() == "identifier" && t.isTypedLocal(leftNode.Utf8Text(t.source)) && t.nodeReturnsJSValue(rightNode) {
							varName := leftNode.Utf8Text(t.source)
							if typeName, ok := t.typedLocalTypes[varName]; ok && isNumericType(typeName) {
								// Numeric type: convert JSValue to number
								rhs = callExpr(selectorExpr(rhs, "Number"))
							} else {
								// String type: convert JSValue to string
								t.addImport("fmt")
								rhs = callExpr(selectorExpr(ident("fmt"), "Sprint"), rhs)
							}
						}
						// When the LHS is a JSValue expression (subscript on JSValue slice,
						// untyped local), convert += to regular assignment with string concat:
						// lhs += rhs → lhs = jsvalue.From(fmt.Sprint(lhs) + fmt.Sprint(rhs))
						if t.nodeReturnsJSValue(leftNode) && opText == "+=" {
							t.addImport("fmt")
							t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
							concat := &ast.BinaryExpr{
								X:  callExpr(selectorExpr(ident("fmt"), "Sprint"), lhs),
								Op: token.ADD,
								Y:  callExpr(selectorExpr(ident("fmt"), "Sprint"), rhs),
							}
							return assignStmt([]ast.Expr{lhs}, []ast.Expr{
								callExpr(selectorExpr(ident("jsvalue"), "From"), concat),
							})
						}
						return &ast.AssignStmt{
							Lhs: []ast.Expr{lhs},
							Tok: mapAugmentedOp(opText),
							Rhs: []ast.Expr{rhs},
						}
					}
				}
			}
			// Update expressions (i++, i--)
			if child.Kind() == "update_expression" {
				argNode := child.ChildByFieldName("argument")
				opNode := child.ChildByFieldName("operator")
				if argNode != nil {
					arg := t.transformExpr(argNode)
					opText := ""
					if opNode != nil {
						opText = opNode.Utf8Text(t.source)
					}
					if arg != nil {
						tok := token.INC
						if opText == "--" {
							tok = token.DEC
						}
						return &ast.IncDecStmt{X: arg, Tok: tok}
					}
				}
			}
			expr := t.transformExpr(child)
			if expr == nil {
				return nil
			}
			// Skip bare nil expression statements (from undefined/null)
			if id, ok := expr.(*ast.Ident); ok && id.Name == "nil" {
				return nil
			}
			// append() used as a statement (from arr.push(x)) must assign back.
			if call, ok := expr.(*ast.CallExpr); ok {
				if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "append" && len(call.Args) > 0 {
					return assignStmt([]ast.Expr{call.Args[0]}, []ast.Expr{call})
				}
			}
			return exprStmt(expr)
		}
		return nil

	case "return_statement":
		var results []ast.Expr
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if e := t.transformExpr(child); e != nil {
				results = append(results, e)
			}
		}
		return returnStmt(results...)

	case "if_statement":
		return t.transformIfStmt(node)

	case "for_statement":
		return t.transformForStmt(node)

	case "for_in_statement":
		return t.transformForInOrOfStmt(node)

	case "for_of_statement":
		return t.transformForInOrOfStmt(node)

	case "while_statement":
		return t.transformWhileStmt(node)

	case "do_statement":
		return t.transformDoWhileStmt(node)

	case "switch_statement":
		return t.transformSwitchStmt(node)

	case "lexical_declaration", "variable_declaration":
		decls := t.transformVarDecl(node)
		if len(decls) == 1 {
			return &ast.DeclStmt{Decl: decls[0]}
		}
		if len(decls) > 1 {
			// Wrap multiple decls — return the first, rest get lost
			// TODO: handle multiple declarations in statement context
			return &ast.DeclStmt{Decl: decls[0]}
		}
		return nil

	case "break_statement":
		return &ast.BranchStmt{Tok: token.BREAK}

	case "continue_statement":
		return &ast.BranchStmt{Tok: token.CONTINUE}

	case "throw_statement":
		return t.transformThrowStmt(node)

	case "try_statement":
		return t.transformTryCatch(node)

	case "block", "statement_block":
		return t.transformBlock(node)

	case "function_declaration":
		// Nested function declarations (JS hoists these within their scope).
		// Go doesn't allow named func decls inside function bodies, so emit
		// as `name := func(...) ... { ... }` variable assignment.
		if d := t.transformFuncDecl(node, false); d != nil {
			funcLit := &ast.FuncLit{
				Type: d.Type,
				Body: d.Body,
			}
			t.addToCurrentScope(d.Name.Name, true)
			return assignDefine([]ast.Expr{ident(d.Name.Name)}, []ast.Expr{funcLit})
		}
		return nil

	case "comment", "line_comment", "block_comment":
		return nil

	default:
		// Try as expression
		if e := t.transformExpr(node); e != nil {
			// Skip bare nil expression statements (from undefined/null/comments)
			if id, ok := e.(*ast.Ident); ok && id.Name == "nil" {
				return nil
			}
			return exprStmt(e)
		}
		return nil
	}
}

func (t *Transformer) transformIfStmt(node *sitter.Node) ast.Stmt {
	condNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("consequence")
	elseNode := node.ChildByFieldName("alternative")

	var cond ast.Expr
	if condNode != nil {
		cond = t.transformExpr(condNode)
		// Unwrap parenthesized expression
		if paren, ok := cond.(*ast.ParenExpr); ok {
			cond = paren.X
		}
		cond = t.ensureBool(cond)
	}
	if cond == nil {
		cond = ident("true")
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = t.transformBlock(bodyNode)
		} else {
			stmt := t.transformStmt(bodyNode)
			if stmt != nil {
				body = blockStmt(stmt)
			} else {
				body = blockStmt()
			}
		}
	} else {
		body = blockStmt()
	}

	ifStmt := &ast.IfStmt{Cond: cond, Body: body}

	if elseNode != nil {
		if elseNode.Kind() == "if_statement" {
			ifStmt.Else = t.transformIfStmt(elseNode)
		} else if elseNode.Kind() == "else_clause" {
			// else clause wraps the body
			if elseNode.NamedChildCount() > 0 {
				elseBody := elseNode.NamedChild(0)
				if elseBody.Kind() == "if_statement" {
					ifStmt.Else = t.transformIfStmt(elseBody)
				} else if elseBody.Kind() == "statement_block" {
					ifStmt.Else = t.transformBlock(elseBody)
				} else {
					stmt := t.transformStmt(elseBody)
					if stmt != nil {
						ifStmt.Else = blockStmt(stmt)
					}
				}
			}
		} else if elseNode.Kind() == "statement_block" {
			ifStmt.Else = t.transformBlock(elseNode)
		} else {
			stmt := t.transformStmt(elseNode)
			if stmt != nil {
				ifStmt.Else = blockStmt(stmt)
			}
		}
	}

	return ifStmt
}

func (t *Transformer) transformForStmt(node *sitter.Node) ast.Stmt {
	initNode := node.ChildByFieldName("initializer")
	condNode := node.ChildByFieldName("condition")
	updateNode := node.ChildByFieldName("increment")
	bodyNode := node.ChildByFieldName("body")

	var init ast.Stmt
	if initNode != nil {
		init = t.transformStmt(initNode)
		// for-loop init must be a simple statement; convert var decls to :=
		if ds, ok := init.(*ast.DeclStmt); ok {
			if gd, ok := ds.Decl.(*ast.GenDecl); ok && len(gd.Specs) == 1 {
				if vs, ok := gd.Specs[0].(*ast.ValueSpec); ok && len(vs.Names) == 1 {
					var rhs ast.Expr
					if len(vs.Values) > 0 {
						rhs = vs.Values[0]
					} else {
						rhs = intLit("0")
					}
					init = assignDefine([]ast.Expr{ident(vs.Names[0].Name)}, []ast.Expr{rhs})
				}
			}
		}
	}

	var cond ast.Expr
	if condNode != nil {
		cond = t.transformExpr(condNode)
	}

	var post ast.Stmt
	if updateNode != nil {
		// update_expression (i++/i--) needs special handling since it's
		// only matched inside expression_statement in transformStmt.
		if updateNode.Kind() == "update_expression" {
			argNode := updateNode.ChildByFieldName("argument")
			opNode := updateNode.ChildByFieldName("operator")
			if argNode != nil {
				// JSValue variables can't use Go's ++ operator.
				// ii++ → ii = jsvalue.NewNumber(ii.Number() + 1)
				if argNode.Kind() == "identifier" && t.isUntypedLocal(argNode.Utf8Text(t.source)) {
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
					lhs := t.transformExpr(argNode)
					rhs := t.transformExpr(argNode)
					op := token.ADD
					if opNode != nil && opNode.Utf8Text(t.source) == "--" {
						op = token.SUB
					}
					post = &ast.AssignStmt{
						Lhs: []ast.Expr{lhs},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{
							callExpr(selectorExpr(ident("jsvalue"), "NewNumber"),
								&ast.BinaryExpr{X: callExpr(selectorExpr(rhs, "Number")), Op: op, Y: intLit("1")}),
						},
					}
				} else if arg := t.transformExpr(argNode); arg != nil {
					tok := token.INC
					if opNode != nil && opNode.Utf8Text(t.source) == "--" {
						tok = token.DEC
					}
					post = &ast.IncDecStmt{X: arg, Tok: tok}
				}
			}
		}
		if post == nil {
			if expr := t.transformExpr(updateNode); expr != nil {
				post = exprStmt(expr)
			}
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = t.transformBlock(bodyNode)
		} else {
			stmt := t.transformStmt(bodyNode)
			body = blockStmt()
			if stmt != nil {
				body.List = append(body.List, stmt)
			}
		}
	} else {
		body = blockStmt()
	}

	return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: body}
}

func (t *Transformer) transformForInOrOfStmt(node *sitter.Node) ast.Stmt {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	bodyNode := node.ChildByFieldName("body")

	// Check operator field to distinguish for-in vs for-of
	isOf := false
	opNode := node.ChildByFieldName("operator")
	if opNode != nil && opNode.Utf8Text(t.source) == "of" {
		isOf = true
	}

	// Extract variable name from left side
	varName := "v"
	var destructurePattern *sitter.Node
	if leftNode != nil {
		switch leftNode.Kind() {
		case "identifier":
			varName = sanitizeIdent(leftNode.Utf8Text(t.source))
		case "object_pattern", "array_pattern":
			// for (const {segment: character} of ...) — left is directly a pattern
			varName = "_item"
			destructurePattern = leftNode
		default:
			// Could be a lexical_declaration wrapping a variable_declarator
			for i := uint(0); i < leftNode.NamedChildCount(); i++ {
				child := leftNode.NamedChild(i)
				if child.Kind() == "variable_declarator" {
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						if nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern" {
							varName = "_item"
							destructurePattern = nameNode
						} else {
							varName = sanitizeIdent(nameNode.Utf8Text(t.source))
						}
					}
				} else if child.Kind() == "identifier" {
					varName = sanitizeIdent(child.Utf8Text(t.source))
				}
			}
		}
	}

	var x ast.Expr = ident("_")
	if rightNode != nil {
		x = t.transformExpr(rightNode)
	}

	var body *ast.BlockStmt
	if bodyNode != nil && bodyNode.Kind() == "statement_block" {
		body = t.transformBlock(bodyNode)
	} else if bodyNode != nil {
		stmt := t.transformStmt(bodyNode)
		body = blockStmt()
		if stmt != nil {
			body.List = append(body.List, stmt)
		}
	} else {
		body = blockStmt()
	}

	// If the loop variable was a destructuring pattern, prepend destructuring
	// statements at the top of the loop body.
	if destructurePattern != nil {
		destructStmts := t.transformDestructuringFromExpr(destructurePattern, ident(varName))
		body.List = append(destructStmts, body.List...)
	}

	if isOf {
		// for (const x of arr) → for _, x := range arr
		return &ast.RangeStmt{
			Key:   ident("_"),
			Value: ident(varName),
			Tok:   token.DEFINE,
			X:     x,
			Body:  body,
		}
	}

	// for (const k in obj) → for k := range obj
	return &ast.RangeStmt{
		Key:  ident(varName),
		Tok:  token.DEFINE,
		X:    x,
		Body: body,
	}
}

func (t *Transformer) transformWhileStmt(node *sitter.Node) ast.Stmt {
	condNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")

	var cond ast.Expr
	if condNode != nil {
		cond = t.transformExpr(condNode)
		if paren, ok := cond.(*ast.ParenExpr); ok {
			cond = paren.X
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil && bodyNode.Kind() == "statement_block" {
		body = t.transformBlock(bodyNode)
	} else if bodyNode != nil {
		stmt := t.transformStmt(bodyNode)
		body = blockStmt()
		if stmt != nil {
			body.List = append(body.List, stmt)
		}
	} else {
		body = blockStmt()
	}

	return &ast.ForStmt{Cond: cond, Body: body}
}

func (t *Transformer) transformDoWhileStmt(node *sitter.Node) ast.Stmt {
	condNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")

	var cond ast.Expr
	if condNode != nil {
		cond = t.transformExpr(condNode)
		if paren, ok := cond.(*ast.ParenExpr); ok {
			cond = paren.X
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil && bodyNode.Kind() == "statement_block" {
		body = t.transformBlock(bodyNode)
	} else if bodyNode != nil {
		stmt := t.transformStmt(bodyNode)
		body = blockStmt()
		if stmt != nil {
			body.List = append(body.List, stmt)
		}
	} else {
		body = blockStmt()
	}

	// do { body } while (cond) → for { body; if !cond { break } }
	breakCheck := &ast.IfStmt{
		Cond: &ast.UnaryExpr{Op: token.NOT, X: cond},
		Body: blockStmt(&ast.BranchStmt{Tok: token.BREAK}),
	}
	body.List = append(body.List, breakCheck)

	return &ast.ForStmt{Body: body}
}

func (t *Transformer) transformSwitchStmt(node *sitter.Node) ast.Stmt {
	tagNode := node.ChildByFieldName("value")
	bodyNode := node.ChildByFieldName("body")

	var tag ast.Expr
	if tagNode != nil {
		tag = t.transformExpr(tagNode)
		if paren, ok := tag.(*ast.ParenExpr); ok {
			tag = paren.X
		}
	}

	var cases []ast.Stmt
	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			child := bodyNode.NamedChild(i)
			if child.Kind() == "switch_case" || child.Kind() == "switch_default" {
				cases = append(cases, t.transformCaseClause(child))
			}
		}
	}

	return &ast.SwitchStmt{
		Tag:  tag,
		Body: &ast.BlockStmt{List: cases},
	}
}

func (t *Transformer) transformCaseClause(node *sitter.Node) ast.Stmt {
	var list []ast.Expr
	var body []ast.Stmt

	valueNode := node.ChildByFieldName("value")
	if valueNode != nil {
		if e := t.transformExpr(valueNode); e != nil {
			list = append(list, e)
		}
	}

	// Collect body statements (skip the "case X:" part)
	foundColon := false
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == ":" {
			foundColon = true
			continue
		}
		if !foundColon {
			continue
		}
		if child.IsNamed() {
			// Skip explicit break statements (Go doesn't need them)
			if child.Kind() == "break_statement" {
				continue
			}
			if s := t.transformStmt(child); s != nil {
				body = append(body, s)
			}
		}
	}

	return &ast.CaseClause{List: list, Body: body}
}

func (t *Transformer) transformThrowStmt(node *sitter.Node) ast.Stmt {
	// throw expr → panic(expr)
	if node.NamedChildCount() > 0 {
		expr := t.transformExpr(node.NamedChild(0))
		if expr != nil {
			return exprStmt(callExpr(ident("panic"), expr))
		}
	}
	return exprStmt(callExpr(ident("panic"), stringLit("unknown error")))
}

func (t *Transformer) transformTryCatch(node *sitter.Node) ast.Stmt {
	bodyNode := node.ChildByFieldName("body")
	handlerNode := node.ChildByFieldName("handler")

	// Simple approach: if the try body is a single expression/assignment with a call,
	// convert to Go error pattern. Otherwise, use defer/recover.

	if bodyNode != nil && bodyNode.NamedChildCount() == 1 {
		inner := bodyNode.NamedChild(0)
		// Check if it's a variable declaration with a function call
		if inner.Kind() == "lexical_declaration" || inner.Kind() == "variable_declaration" {
			decls := t.transformVarDecl(inner)
			if len(decls) == 1 {
				// Just emit the declaration; error handling is complex
				return &ast.DeclStmt{Decl: decls[0]}
			}
		}
	}

	// General case: wrap in func with defer/recover
	var tryBody *ast.BlockStmt
	if bodyNode != nil {
		tryBody = t.transformBlock(bodyNode)
	} else {
		tryBody = blockStmt()
	}

	if handlerNode == nil {
		return tryBody
	}

	// Build: defer func() { if r := recover(); r != nil { <catch body> } }()
	catchBody := blockStmt()
	catchBodyNode := handlerNode.ChildByFieldName("body")
	if catchBodyNode != nil {
		catchBody = t.transformBlock(catchBodyNode)
	}

	// Strip return statements from catch body — they can't return from the
	// enclosing function when inside a defer func() closure.
	stripReturns(catchBody)

	// Get catch parameter name
	catchParam := "r"
	catchParamNode := handlerNode.ChildByFieldName("parameter")
	if catchParamNode != nil {
		// parameter might be a catch_clause parameter like (e) — extract the identifier
		text := catchParamNode.Utf8Text(t.source)
		text = strings.TrimSpace(text)
		if text != "" {
			catchParam = text
		}
	}

	recoverCall := callExpr(ident("recover"))
	recoverAssign := assignDefine(
		[]ast.Expr{ident(catchParam)},
		[]ast.Expr{recoverCall},
	)

	ifRecover := &ast.IfStmt{
		Init: recoverAssign,
		Cond: &ast.BinaryExpr{
			X:  ident(catchParam),
			Op: token.NEQ,
			Y:  ident("nil"),
		},
		Body: catchBody,
	}

	deferFunc := &ast.DeferStmt{
		Call: &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList()},
				Body: blockStmt(ifRecover),
			},
		},
	}

	// Combine: defer + try body
	result := blockStmt()
	result.List = append(result.List, deferFunc)
	result.List = append(result.List, tryBody.List...)
	return result
}

// stripReturns removes return statements from a block, replacing them with
// bare returns. This is needed when catch bodies are placed inside defer funcs
// where returning a value would return from the closure, not the outer function.
func stripReturns(block *ast.BlockStmt) {
	for i, stmt := range block.List {
		if ret, ok := stmt.(*ast.ReturnStmt); ok {
			ret.Results = nil
			block.List[i] = ret
		}
		// Recurse into nested blocks
		switch s := stmt.(type) {
		case *ast.IfStmt:
			if s.Body != nil {
				stripReturns(s.Body)
			}
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				stripReturns(elseBlock)
			}
		case *ast.BlockStmt:
			stripReturns(s)
		}
	}
}
