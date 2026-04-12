package hir

import (
	"github.com/nnstd/gun/compiler/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// --------------------------------------------------------------------
// Statements
// --------------------------------------------------------------------

func (b *Builder) buildBlock(node *sitter.Node) *BlockStmt {
	if node == nil {
		return &BlockStmt{}
	}
	block := &BlockStmt{Span: b.span(node)}
	b.symtab.PushScope()
	defer b.symtab.PopScope()

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if s := b.buildStmt(child); s != nil {
			block.Stmts = append(block.Stmts, s)
		}
	}
	return block
}

func (b *Builder) buildStmt(node *sitter.Node) Stmt {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "expression_statement":
		if node.NamedChildCount() > 0 {
			expr := b.buildExpr(node.NamedChild(0))
			if expr != nil {
				return &ExprStmt{Expr: expr, Span: b.span(node)}
			}
		}
		return nil

	case "return_statement":
		var value Expr
		if node.NamedChildCount() > 0 {
			value = b.buildExpr(node.NamedChild(0))
		}
		return &ReturnStmt{Value: value, Span: b.span(node)}

	case "if_statement":
		return b.buildIfStmt(node)

	case "for_statement":
		return b.buildForStmt(node)

	case "for_in_statement":
		// tree-sitter uses for_in_statement for both for-in and for-of.
		// Distinguish by checking the operator field.
		if b.isForOf(node) {
			return b.buildForOfStmt(node)
		}
		return b.buildForInStmt(node)

	case "while_statement":
		return b.buildWhileStmt(node)

	case "do_statement":
		return b.buildDoWhileStmt(node)

	case "switch_statement":
		return b.buildSwitchStmt(node)

	case "try_statement":
		return b.buildTryCatchStmt(node)

	case "throw_statement":
		var value Expr
		if node.NamedChildCount() > 0 {
			value = b.buildExpr(node.NamedChild(0))
		}
		return &ThrowStmt{Value: value, Span: b.span(node)}

	case "break_statement":
		label := ""
		if node.NamedChildCount() > 0 {
			label = b.nodeText(node.NamedChild(0))
		}
		return &BreakStmt{Label: label}

	case "continue_statement":
		label := ""
		if node.NamedChildCount() > 0 {
			label = b.nodeText(node.NamedChild(0))
		}
		return &ContinueStmt{Label: label}

	case "labeled_statement":
		label := ""
		if labelNode := node.ChildByFieldName("label"); labelNode != nil {
			label = b.nodeText(labelNode)
		}
		var inner Stmt
		if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
			inner = b.buildStmt(bodyNode)
		}
		return &LabeledStmt{Label: label, Stmt: inner}

	case "lexical_declaration", "variable_declaration":
		return b.buildVarDecl(node, false)

	case "statement_block", "block":
		return b.buildBlock(node)

	case "empty_statement":
		return &EmptyStmt{}

	case "function_declaration":
		// Function declaration inside a block — treat as local function variable
		if d := b.buildFuncDecl(node, false); d != nil {
			// Convert to a VarDecl with function value
			return &VarDecl{
				Kind: VarLet,
				Declarators: []*Declarator{{
					Symbol: d.Symbol,
					Init: &ArrowFunc{
						Params: d.Params,
						Body:   d.Body,
						Span:   d.Span,
					},
				}},
			}
		}
		return nil

	case "comment", "line_comment", "block_comment":
		return nil

	default:
		// Try to parse as expression
		expr := b.buildExpr(node)
		if expr != nil {
			return &ExprStmt{Expr: expr, Span: b.span(node)}
		}
		return nil
	}
}

func (b *Builder) buildIfStmt(node *sitter.Node) *IfStmt {
	condNode := node.ChildByFieldName("condition")
	consNode := node.ChildByFieldName("consequence")
	altNode := node.ChildByFieldName("alternative")

	var cond Expr
	if condNode != nil {
		cond = b.buildExpr(condNode)
	}

	var then *BlockStmt
	if consNode != nil {
		if consNode.Kind() == "statement_block" {
			then = b.buildBlock(consNode)
		} else {
			s := b.buildStmt(consNode)
			if s != nil {
				then = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if then == nil {
		then = &BlockStmt{}
	}

	var elseStmt Stmt
	if altNode != nil {
		if altNode.Kind() == "else_clause" {
			if altNode.NamedChildCount() > 0 {
				inner := altNode.NamedChild(0)
				if inner.Kind() == "if_statement" {
					elseStmt = b.buildIfStmt(inner)
				} else {
					elseStmt = b.buildStmt(inner)
				}
			}
		} else {
			elseStmt = b.buildStmt(altNode)
		}
	}

	return &IfStmt{
		Cond: cond,
		Then: then,
		Else: elseStmt,
		Span: b.span(node),
	}
}

func (b *Builder) buildForStmt(node *sitter.Node) *ForStmt {
	initNode := node.ChildByFieldName("initializer")
	condNode := node.ChildByFieldName("condition")
	updateNode := node.ChildByFieldName("increment")
	bodyNode := node.ChildByFieldName("body")

	var init Stmt
	if initNode != nil && initNode.IsNamed() && initNode.Kind() != "empty_statement" {
		switch initNode.Kind() {
		case "lexical_declaration", "variable_declaration":
			init = b.buildVarDecl(initNode, false)
		default:
			expr := b.buildExpr(initNode)
			if expr != nil {
				init = &ExprStmt{Expr: expr}
			}
		}
	}

	var cond Expr
	if condNode != nil && condNode.IsNamed() && condNode.Kind() != "empty_statement" {
		cond = b.buildExpr(condNode)
	}

	var post Expr
	if updateNode != nil && updateNode.IsNamed() && updateNode.Kind() != "empty_statement" {
		post = b.buildExpr(updateNode)
	}

	var body *BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = b.buildBlock(bodyNode)
		} else {
			s := b.buildStmt(bodyNode)
			if s != nil {
				body = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if body == nil {
		body = &BlockStmt{}
	}

	return &ForStmt{Init: init, Cond: cond, Post: post, Body: body, Span: b.span(node)}
}

func (b *Builder) buildForInStmt(node *sitter.Node) *ForInStmt {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	bodyNode := node.ChildByFieldName("body")

	var key *symbol.Symbol
	if leftNode != nil {
		name := b.extractVarName(leftNode)
		if name != "" {
			key = b.symtab.Define(name, symbol.KindVariable)
		}
	}

	var value Expr
	if rightNode != nil {
		value = b.buildExpr(rightNode)
	}

	var body *BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = b.buildBlock(bodyNode)
		} else {
			s := b.buildStmt(bodyNode)
			if s != nil {
				body = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if body == nil {
		body = &BlockStmt{}
	}

	return &ForInStmt{Key: key, Value: value, Body: body, Span: b.span(node)}
}

func (b *Builder) buildForOfStmt(node *sitter.Node) *ForOfStmt {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	bodyNode := node.ChildByFieldName("body")

	stmt := &ForOfStmt{}

	if leftNode != nil {
		switch leftNode.Kind() {
		case "array_pattern":
			stmt.Pattern = b.buildArrayPattern(leftNode)
		case "object_pattern":
			stmt.Pattern = b.buildObjectPattern(leftNode)
		default:
			// Could be a simple variable or lexical_declaration wrapping a pattern
			varName := b.extractVarName(leftNode)
			if varName != "" {
				stmt.Elem = b.symtab.Define(varName, symbol.KindVariable)
			} else {
				// Unwrap lexical_declaration/variable_declaration
				inner := leftNode
				if inner.Kind() == "lexical_declaration" || inner.Kind() == "variable_declaration" {
					for i := uint(0); i < inner.NamedChildCount(); i++ {
						child := inner.NamedChild(i)
						if child.Kind() == "variable_declarator" {
							nameNode := child.ChildByFieldName("name")
							if nameNode != nil {
								if nameNode.Kind() == "object_pattern" {
									stmt.Pattern = b.buildObjectPattern(nameNode)
								} else if nameNode.Kind() == "array_pattern" {
									stmt.Pattern = b.buildArrayPattern(nameNode)
								}
							}
							break
						}
					}
				}
			}
		}
	}

	if rightNode != nil {
		stmt.Value = b.buildExpr(rightNode)
	}

	var body *BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = b.buildBlock(bodyNode)
		} else {
			s := b.buildStmt(bodyNode)
			if s != nil {
				body = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if body == nil {
		body = &BlockStmt{}
	}
	stmt.Body = body
	stmt.Span = b.span(node)

	return stmt
}

func (b *Builder) buildWhileStmt(node *sitter.Node) *WhileStmt {
	condNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")

	var cond Expr
	if condNode != nil {
		cond = b.buildExpr(condNode)
	}

	var body *BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = b.buildBlock(bodyNode)
		} else {
			s := b.buildStmt(bodyNode)
			if s != nil {
				body = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if body == nil {
		body = &BlockStmt{}
	}

	return &WhileStmt{Cond: cond, Body: body, Span: b.span(node)}
}

func (b *Builder) buildDoWhileStmt(node *sitter.Node) *DoWhileStmt {
	bodyNode := node.ChildByFieldName("body")
	condNode := node.ChildByFieldName("condition")

	var cond Expr
	if condNode != nil {
		cond = b.buildExpr(condNode)
	}

	var body *BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = b.buildBlock(bodyNode)
		} else {
			s := b.buildStmt(bodyNode)
			if s != nil {
				body = &BlockStmt{Stmts: []Stmt{s}}
			}
		}
	}
	if body == nil {
		body = &BlockStmt{}
	}

	return &DoWhileStmt{Body: body, Cond: cond, Span: b.span(node)}
}

func (b *Builder) buildSwitchStmt(node *sitter.Node) *SwitchStmt {
	valueNode := node.ChildByFieldName("value")
	bodyNode := node.ChildByFieldName("body")

	var tag Expr
	if valueNode != nil {
		tag = b.buildExpr(valueNode)
	}

	var cases []*CaseClause
	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			child := bodyNode.NamedChild(i)
			switch child.Kind() {
			case "switch_case":
				cc := &CaseClause{}
				valueNode := child.ChildByFieldName("value")
				if valueNode != nil {
					cc.Value = b.buildExpr(valueNode)
				}
				for j := uint(0); j < child.NamedChildCount(); j++ {
					stmtNode := child.NamedChild(j)
					if stmtNode == valueNode {
						continue
					}
					if s := b.buildStmt(stmtNode); s != nil {
						cc.Body = append(cc.Body, s)
					}
				}
				cases = append(cases, cc)
			case "switch_default":
				cc := &CaseClause{} // Value = nil means default
				for j := uint(0); j < child.NamedChildCount(); j++ {
					if s := b.buildStmt(child.NamedChild(j)); s != nil {
						cc.Body = append(cc.Body, s)
					}
				}
				cases = append(cases, cc)
			}
		}
	}

	return &SwitchStmt{Tag: tag, Cases: cases, Span: b.span(node)}
}

func (b *Builder) buildTryCatchStmt(node *sitter.Node) *TryCatchStmt {
	bodyNode := node.ChildByFieldName("body")
	handlerNode := node.ChildByFieldName("handler")
	finalizerNode := node.ChildByFieldName("finalizer")

	stmt := &TryCatchStmt{}

	if bodyNode != nil {
		stmt.Try = b.buildBlock(bodyNode)
	}

	if handlerNode != nil {
		catch := &CatchClause{}
		paramNode := handlerNode.ChildByFieldName("parameter")
		if paramNode != nil {
			name := b.nodeText(paramNode)
			catch.Param = b.symtab.Define(name, symbol.KindVariable)
		}
		catchBody := handlerNode.ChildByFieldName("body")
		if catchBody != nil {
			catch.Body = b.buildBlock(catchBody)
		}
		stmt.Catch = catch
	}

	if finalizerNode != nil {
		finallyBody := finalizerNode.ChildByFieldName("body")
		if finallyBody != nil {
			stmt.Finally = b.buildBlock(finallyBody)
		} else {
			stmt.Finally = b.buildBlock(finalizerNode)
		}
	}

	stmt.Span = b.span(node)
	return stmt
}

// isForOf checks if a for_in_statement is actually a for-of by inspecting
// the operator field.
func (b *Builder) isForOf(node *sitter.Node) bool {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "of" {
			return true
		}
		// Also check the operator field name
		if node.FieldNameForChild(uint32(i)) == "operator" && b.nodeText(child) == "of" {
			return true
		}
	}
	return false
}

// extractVarName extracts the variable name from a for-in/for-of left side.
func (b *Builder) extractVarName(node *sitter.Node) string {
	switch node.Kind() {
	case "identifier":
		return b.nodeText(node)
	case "lexical_declaration", "variable_declaration":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "variable_declarator" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil && nameNode.Kind() == "identifier" {
					return b.nodeText(nameNode)
				}
			}
		}
	}
	return ""
}
