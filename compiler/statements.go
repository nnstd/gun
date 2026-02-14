package compiler

import (
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

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
					lhs := t.transformExpr(leftNode)
					rhs := t.transformExpr(rightNode)
					if lhs != nil && rhs != nil {
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

	default:
		// Try as expression
		if e := t.transformExpr(node); e != nil {
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
		expr := t.transformExpr(updateNode)
		if expr != nil {
			post = exprStmt(expr)
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
	if leftNode != nil {
		switch leftNode.Kind() {
		case "identifier":
			varName = leftNode.Utf8Text(t.source)
		default:
			// Could be a lexical_declaration wrapping a variable_declarator
			for i := uint(0); i < leftNode.NamedChildCount(); i++ {
				child := leftNode.NamedChild(i)
				if child.Kind() == "variable_declarator" {
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						varName = nameNode.Utf8Text(t.source)
					}
				} else if child.Kind() == "identifier" {
					varName = child.Utf8Text(t.source)
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
