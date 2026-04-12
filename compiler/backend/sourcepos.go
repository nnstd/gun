package backend

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/nnstd/gun/compiler/hir"
)

const sourcePosLineStride = 4096
const lineMarkerFunc = "__gun_line"

func sourcePos(span *hir.SourceSpan) token.Pos {
	if span == nil || span.StartLine <= 0 {
		return token.NoPos
	}
	col := span.StartColumn
	if col <= 0 {
		col = 1
	}
	return token.Pos((span.StartLine-1)*sourcePosLineStride + col)
}

func sourceEndPos(span *hir.SourceSpan) token.Pos {
	if span == nil {
		return token.NoPos
	}
	line := span.EndLine
	col := span.EndColumn
	if line <= 0 {
		line = span.StartLine
	}
	if col <= 0 {
		col = span.StartColumn + 1
	}
	return token.Pos((line-1)*sourcePosLineStride + col)
}

func hirStmtSpan(stmt hir.Stmt) *hir.SourceSpan {
	switch stmt := stmt.(type) {
	case *hir.BlockStmt:
		return stmt.Span
	case *hir.ExprStmt:
		return stmt.Span
	case *hir.ReturnStmt:
		return stmt.Span
	case *hir.IfStmt:
		return stmt.Span
	case *hir.ForStmt:
		return stmt.Span
	case *hir.ForInStmt:
		return stmt.Span
	case *hir.ForOfStmt:
		return stmt.Span
	case *hir.WhileStmt:
		return stmt.Span
	case *hir.DoWhileStmt:
		return stmt.Span
	case *hir.SwitchStmt:
		return stmt.Span
	case *hir.TryCatchStmt:
		return stmt.Span
	case *hir.ThrowStmt:
		return stmt.Span
	default:
		return nil
	}
}

func hirExprSpan(expr hir.Expr) *hir.SourceSpan {
	switch expr := expr.(type) {
	case *hir.AssignExpr:
		return expr.Span
	case *hir.CallExpr:
		return expr.Span
	case *hir.NewExpr:
		return expr.Span
	case *hir.MemberExpr:
		return expr.Span
	case *hir.ComputedMemberExpr:
		return expr.Span
	case *hir.ArrowFunc:
		return expr.Span
	case *hir.FuncExpr:
		return expr.Span
	case *hir.SequenceExpr:
		return expr.Span
	case *hir.ClassExpr:
		return expr.Span
	default:
		return nil
	}
}

func setFuncDeclPos(fn *ast.FuncDecl, span *hir.SourceSpan) *ast.FuncDecl {
	if fn == nil || span == nil {
		return fn
	}
	pos := sourcePos(span)
	if fn.Type != nil && fn.Type.Func == token.NoPos {
		fn.Type.Func = pos
	}
	if fn.Name != nil && fn.Name.NamePos == token.NoPos {
		fn.Name.NamePos = pos
	}
	setBlockPos(fn.Body, span)
	return fn
}

func setFuncLitPos(fn *ast.FuncLit, span *hir.SourceSpan) *ast.FuncLit {
	if fn == nil || span == nil {
		return fn
	}
	pos := sourcePos(span)
	if fn.Type != nil && fn.Type.Func == token.NoPos {
		fn.Type.Func = pos
	}
	setBlockPos(fn.Body, span)
	return fn
}

func setBlockPos(block *ast.BlockStmt, span *hir.SourceSpan) *ast.BlockStmt {
	if block == nil || span == nil {
		return block
	}
	if block.Lbrace == token.NoPos {
		block.Lbrace = sourcePos(span)
	}
	if block.Rbrace == token.NoPos {
		block.Rbrace = sourceEndPos(span)
	}
	return block
}

func setDeclPos(decl ast.Decl, span *hir.SourceSpan) ast.Decl {
	if decl == nil || span == nil {
		return decl
	}
	switch decl := decl.(type) {
	case *ast.FuncDecl:
		return setFuncDeclPos(decl, span)
	case *ast.GenDecl:
		if decl.TokPos == token.NoPos {
			decl.TokPos = sourcePos(span)
		}
	}
	return decl
}

func setStmtPos(stmt ast.Stmt, span *hir.SourceSpan) ast.Stmt {
	if stmt == nil || span == nil {
		return stmt
	}
	pos := sourcePos(span)
	switch stmt := stmt.(type) {
	case *ast.BlockStmt:
		setBlockPos(stmt, span)
	case *ast.ExprStmt:
		stmt.X = setExprPos(stmt.X, span)
	case *ast.ReturnStmt:
		if stmt.Return == token.NoPos {
			stmt.Return = pos
		}
		for i, result := range stmt.Results {
			stmt.Results[i] = setExprPos(result, span)
		}
	case *ast.AssignStmt:
		if stmt.TokPos == token.NoPos {
			stmt.TokPos = pos
		}
		for i, expr := range stmt.Lhs {
			stmt.Lhs[i] = setExprPos(expr, span)
		}
		for i, expr := range stmt.Rhs {
			stmt.Rhs[i] = setExprPos(expr, span)
		}
	case *ast.IfStmt:
		if stmt.If == token.NoPos {
			stmt.If = pos
		}
		stmt.Cond = setExprPos(stmt.Cond, span)
		setBlockPos(stmt.Body, span)
		stmt.Else = setStmtPos(stmt.Else, span)
	case *ast.ForStmt:
		if stmt.For == token.NoPos {
			stmt.For = pos
		}
		stmt.Init = setStmtPos(stmt.Init, span)
		stmt.Cond = setExprPos(stmt.Cond, span)
		stmt.Post = setStmtPos(stmt.Post, span)
		setBlockPos(stmt.Body, span)
	case *ast.RangeStmt:
		if stmt.For == token.NoPos {
			stmt.For = pos
		}
		stmt.X = setExprPos(stmt.X, span)
		setBlockPos(stmt.Body, span)
	case *ast.SwitchStmt:
		if stmt.Switch == token.NoPos {
			stmt.Switch = pos
		}
		stmt.Tag = setExprPos(stmt.Tag, span)
		setBlockPos(stmt.Body, span)
	case *ast.CaseClause:
		if stmt.Case == token.NoPos {
			stmt.Case = pos
		}
		for i, expr := range stmt.List {
			stmt.List[i] = setExprPos(expr, span)
		}
		for i, child := range stmt.Body {
			stmt.Body[i] = setStmtPos(child, span)
		}
	case *ast.BranchStmt:
		if stmt.TokPos == token.NoPos {
			stmt.TokPos = pos
		}
	case *ast.DeclStmt:
		stmt.Decl = setDeclPos(stmt.Decl, span)
	case *ast.LabeledStmt:
		if stmt.Label != nil && stmt.Label.NamePos == token.NoPos {
			stmt.Label.NamePos = pos
		}
		stmt.Stmt = setStmtPos(stmt.Stmt, span)
	}
	return stmt
}

func setExprPos(expr ast.Expr, span *hir.SourceSpan) ast.Expr {
	if expr == nil || span == nil {
		return expr
	}
	pos := sourcePos(span)
	switch expr := expr.(type) {
	case *ast.Ident:
		if expr.NamePos == token.NoPos {
			expr.NamePos = pos
		}
	case *ast.BasicLit:
		if expr.ValuePos == token.NoPos {
			expr.ValuePos = pos
		}
	case *ast.CallExpr:
		expr.Fun = setExprPos(expr.Fun, span)
		if expr.Lparen == token.NoPos {
			expr.Lparen = pos
		}
		for i, arg := range expr.Args {
			expr.Args[i] = setExprPos(arg, span)
		}
	case *ast.SelectorExpr:
		expr.X = setExprPos(expr.X, span)
		if expr.Sel != nil && expr.Sel.NamePos == token.NoPos {
			expr.Sel.NamePos = pos
		}
	case *ast.FuncLit:
		setFuncLitPos(expr, span)
	case *ast.FuncType:
		if expr.Func == token.NoPos {
			expr.Func = pos
		}
	case *ast.ParenExpr:
		if expr.Lparen == token.NoPos {
			expr.Lparen = pos
		}
		if expr.Rparen == token.NoPos {
			expr.Rparen = sourceEndPos(span)
		}
		expr.X = setExprPos(expr.X, span)
	case *ast.UnaryExpr:
		if expr.OpPos == token.NoPos {
			expr.OpPos = pos
		}
		expr.X = setExprPos(expr.X, span)
	case *ast.BinaryExpr:
		expr.X = setExprPos(expr.X, span)
		expr.Y = setExprPos(expr.Y, span)
		if expr.OpPos == token.NoPos {
			expr.OpPos = pos
		}
	case *ast.IndexExpr:
		expr.X = setExprPos(expr.X, span)
		expr.Index = setExprPos(expr.Index, span)
		if expr.Lbrack == token.NoPos {
			expr.Lbrack = pos
		}
		if expr.Rbrack == token.NoPos {
			expr.Rbrack = sourceEndPos(span)
		}
	case *ast.SliceExpr:
		expr.X = setExprPos(expr.X, span)
		if expr.Low != nil {
			expr.Low = setExprPos(expr.Low, span)
		}
		if expr.High != nil {
			expr.High = setExprPos(expr.High, span)
		}
		if expr.Max != nil {
			expr.Max = setExprPos(expr.Max, span)
		}
		if expr.Lbrack == token.NoPos {
			expr.Lbrack = pos
		}
		if expr.Rbrack == token.NoPos {
			expr.Rbrack = sourceEndPos(span)
		}
	case *ast.CompositeLit:
		if expr.Type != nil {
			expr.Type = setExprPos(expr.Type, span)
		}
		if expr.Lbrace == token.NoPos {
			expr.Lbrace = pos
		}
		if expr.Rbrace == token.NoPos {
			expr.Rbrace = sourceEndPos(span)
		}
		for i, elt := range expr.Elts {
			expr.Elts[i] = setExprPos(elt, span)
		}
	case *ast.KeyValueExpr:
		expr.Key = setExprPos(expr.Key, span)
		expr.Value = setExprPos(expr.Value, span)
		if expr.Colon == token.NoPos {
			expr.Colon = pos
		}
	case *ast.StarExpr:
		if expr.Star == token.NoPos {
			expr.Star = pos
		}
		expr.X = setExprPos(expr.X, span)
	case *ast.ArrayType:
		if expr.Lbrack == token.NoPos {
			expr.Lbrack = pos
		}
		expr.Elt = setExprPos(expr.Elt, span)
	case *ast.Ellipsis:
		if expr.Ellipsis == token.NoPos {
			expr.Ellipsis = pos
		}
		expr.Elt = setExprPos(expr.Elt, span)
	}
	return expr
}

func (l *Lowerer) lineDirectiveMarker(span *hir.SourceSpan) ast.Stmt {
	if span == nil || span.StartLine <= 0 || l.sourcePath == "" {
		return nil
	}
	return exprStmt(callExpr(
		goIdent(lineMarkerFunc),
		stringLit(l.sourcePath),
		intLit(strconv.Itoa(span.StartLine)),
	))
}

func (l *Lowerer) appendWithLineMarker(stmts []ast.Stmt, span *hir.SourceSpan, stmt ast.Stmt) []ast.Stmt {
	if marker := l.lineDirectiveMarker(span); marker != nil {
		stmts = append(stmts, marker)
	}
	return append(stmts, stmt)
}
