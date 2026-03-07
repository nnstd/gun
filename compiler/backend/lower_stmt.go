package backend

import (
	"go/ast"
	"go/token"

	"github.com/nnstd/gun/compiler/hir"
)

// --------------------------------------------------------------------
// Statements
// --------------------------------------------------------------------

func (l *Lowerer) lowerBlock(b *hir.BlockStmt) *ast.BlockStmt {
	if b == nil {
		return blockStmt()
	}
	var stmts []ast.Stmt
	for _, s := range b.Stmts {
		if gs := l.lowerStmt(s); gs != nil {
			stmts = append(stmts, gs)
		}
	}
	return &ast.BlockStmt{List: stmts}
}

func (l *Lowerer) lowerStmt(s hir.Stmt) ast.Stmt {
	if s == nil {
		return nil
	}
	switch s := s.(type) {
	case *hir.ExprStmt:
		expr := l.lowerExpr(s.Expr)
		if expr == nil {
			return nil
		}
		return exprStmt(expr)

	case *hir.ReturnStmt:
		if s.Value != nil {
			val := l.lowerExpr(s.Value)
			val = jsvalueWrapLit(val)
			return returnStmt(val)
		}
		return returnStmt()

	case *hir.BlockStmt:
		return l.lowerBlock(s)

	case *hir.VarDecl:
		return l.lowerLocalVarDecl(s)

	case *hir.IfStmt:
		return l.lowerIfStmt(s)

	case *hir.ForStmt:
		return l.lowerForStmt(s)

	case *hir.ForInStmt:
		return l.lowerForInStmt(s)

	case *hir.ForOfStmt:
		return l.lowerForOfStmt(s)

	case *hir.WhileStmt:
		return l.lowerWhileStmt(s)

	case *hir.DoWhileStmt:
		return l.lowerDoWhileStmt(s)

	case *hir.SwitchStmt:
		return l.lowerSwitchStmt(s)

	case *hir.TryCatchStmt:
		return l.lowerTryCatchStmt(s)

	case *hir.ThrowStmt:
		val := l.lowerExpr(s.Value)
		return exprStmt(callExpr(goIdent("panic"), val))

	case *hir.BreakStmt:
		bs := &ast.BranchStmt{Tok: token.BREAK}
		if s.Label != "" {
			bs.Label = goIdent(s.Label)
		}
		return bs

	case *hir.ContinueStmt:
		cs := &ast.BranchStmt{Tok: token.CONTINUE}
		if s.Label != "" {
			cs.Label = goIdent(s.Label)
		}
		return cs

	case *hir.LabeledStmt:
		return &ast.LabeledStmt{
			Label: goIdent(s.Label),
			Stmt:  l.lowerStmt(s.Stmt),
		}

	case *hir.EmptyStmt:
		return &ast.EmptyStmt{}

	default:
		return nil
	}
}

func (l *Lowerer) lowerLocalVarDecl(d *hir.VarDecl) ast.Stmt {
	// Local variable declarations become := assignments
	for _, decl := range d.Declarators {
		if decl.Symbol == nil {
			continue
		}
		name := l.emitName(decl.Symbol)
		var value ast.Expr
		if decl.Init != nil {
			value = l.lowerExpr(decl.Init)
			value = jsvalueWrapLit(value)
		} else {
			value = goIdent("nil")
		}
		return assignDefine(
			[]ast.Expr{goIdent(name)},
			[]ast.Expr{value},
		)
	}
	return nil
}

func (l *Lowerer) lowerIfStmt(s *hir.IfStmt) *ast.IfStmt {
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)

	then := l.lowerBlock(s.Then)

	var elseStmt ast.Stmt
	if s.Else != nil {
		elseStmt = l.lowerStmt(s.Else)
	}

	return &ast.IfStmt{
		Cond: cond,
		Body: then,
		Else: elseStmt,
	}
}

func (l *Lowerer) lowerForStmt(s *hir.ForStmt) *ast.ForStmt {
	var init ast.Stmt
	if s.Init != nil {
		init = l.lowerStmt(s.Init)
	}

	var cond ast.Expr
	if s.Cond != nil {
		cond = l.lowerExpr(s.Cond)
		cond = l.ensureBool(cond)
	}

	var post ast.Stmt
	if s.Post != nil {
		postExpr := l.lowerExpr(s.Post)
		if postExpr != nil {
			post = exprStmt(postExpr)
		}
	}

	body := l.lowerBlock(s.Body)

	return &ast.ForStmt{
		Init: init,
		Cond: cond,
		Post: post,
		Body: body,
	}
}

func (l *Lowerer) lowerForInStmt(s *hir.ForInStmt) ast.Stmt {
	l.jsvalueImport()

	keyName := "_"
	if s.Key != nil {
		keyName = l.emitName(s.Key)
	}

	value := l.lowerExpr(s.Value)
	body := l.lowerBlock(s.Body)

	// for _, key := range obj.OwnKeys()
	return &ast.RangeStmt{
		Key:   goIdent("_"),
		Value: goIdent(keyName),
		Tok:   token.DEFINE,
		X:     callExpr(selectorExpr(value, "OwnKeys")),
		Body:  body,
	}
}

func (l *Lowerer) lowerForOfStmt(s *hir.ForOfStmt) ast.Stmt {
	l.jsvalueImport()

	elemName := "_"
	if s.Elem != nil {
		elemName = l.emitName(s.Elem)
	}

	value := l.lowerExpr(s.Value)
	body := l.lowerBlock(s.Body)

	// for _, elem := range arr.Array()
	return &ast.RangeStmt{
		Key:   goIdent("_"),
		Value: goIdent(elemName),
		Tok:   token.DEFINE,
		X:     callExpr(selectorExpr(value, "Array")),
		Body:  body,
	}
}

func (l *Lowerer) lowerWhileStmt(s *hir.WhileStmt) *ast.ForStmt {
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)
	body := l.lowerBlock(s.Body)

	return &ast.ForStmt{Cond: cond, Body: body}
}

func (l *Lowerer) lowerDoWhileStmt(s *hir.DoWhileStmt) *ast.ForStmt {
	body := l.lowerBlock(s.Body)
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)

	// for { body; if !cond { break } }
	body.List = append(body.List, &ast.IfStmt{
		Cond: &ast.UnaryExpr{Op: token.NOT, X: cond},
		Body: blockStmt(&ast.BranchStmt{Tok: token.BREAK}),
	})

	return &ast.ForStmt{Body: body}
}

func (l *Lowerer) lowerSwitchStmt(s *hir.SwitchStmt) *ast.SwitchStmt {
	tag := l.lowerExpr(s.Tag)

	var cases []ast.Stmt
	for _, c := range s.Cases {
		cc := &ast.CaseClause{}
		if c.Value != nil {
			cc.List = []ast.Expr{l.lowerExpr(c.Value)}
		}
		for _, st := range c.Body {
			if gs := l.lowerStmt(st); gs != nil {
				cc.Body = append(cc.Body, gs)
			}
		}
		cases = append(cases, cc)
	}

	return &ast.SwitchStmt{
		Tag:  tag,
		Body: &ast.BlockStmt{List: cases},
	}
}

func (l *Lowerer) lowerTryCatchStmt(s *hir.TryCatchStmt) ast.Stmt {
	l.jsvalueImport()

	var stmts []ast.Stmt

	// Build defer+recover for catch
	if s.Catch != nil {
		catchBody := l.lowerBlock(s.Catch.Body)

		paramName := "_"
		if s.Catch.Param != nil {
			paramName = l.emitName(s.Catch.Param)
		}

		// Prepend: paramName := jsvalue.From(r)
		catchBody.List = append([]ast.Stmt{
			assignDefine(
				[]ast.Expr{goIdent(paramName)},
				[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "From"), goIdent("r"))},
			),
		}, catchBody.List...)

		recoverBlock := blockStmt(
			&ast.IfStmt{
				Init: assignDefine(
					[]ast.Expr{goIdent("r")},
					[]ast.Expr{callExpr(goIdent("recover"))},
				),
				Cond: &ast.BinaryExpr{
					X: goIdent("r"), Op: token.NEQ, Y: goIdent("nil"),
				},
				Body: catchBody,
			},
		)

		deferStmt := &ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Params: fieldList()},
					Body: recoverBlock,
				},
			},
		}
		stmts = append(stmts, deferStmt)
	}

	// Try body
	if s.Try != nil {
		for _, st := range s.Try.Stmts {
			if gs := l.lowerStmt(st); gs != nil {
				stmts = append(stmts, gs)
			}
		}
	}

	// Finally is lowered as a separate defer
	if s.Finally != nil {
		finallyBody := l.lowerBlock(s.Finally)
		finallyDefer := &ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Params: fieldList()},
					Body: finallyBody,
				},
			},
		}
		// Finally defer should be first (executes last)
		stmts = append([]ast.Stmt{finallyDefer}, stmts...)
	}

	return &ast.BlockStmt{List: stmts}
}

// ensureBool wraps a JSValue expression with .Bool() for use in Go conditions.
func (l *Lowerer) ensureBool(expr ast.Expr) ast.Expr {
	if expr == nil {
		return goIdent("true")
	}

	// Already a native bool expression
	if l.isNativeBool(expr) {
		return expr
	}

	// JSValue expression → .Bool()
	return callExpr(selectorExpr(expr, "Bool"))
}

// isNativeBool returns true if the expression produces a native Go bool.
func (l *Lowerer) isNativeBool(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ,
			token.LAND, token.LOR:
			return true
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return true
		}
	}
	return false
}
