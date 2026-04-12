package hir

import (
	"fmt"
	"strings"
)

// Diagnostic is a source-located validation problem.
type Diagnostic struct {
	SourcePath string
	Span       *SourceSpan
	Message    string
}

// DiagnosticError aggregates one or more validation diagnostics.
type DiagnosticError struct {
	Diagnostics []Diagnostic
}

func (e *DiagnosticError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return ""
	}
	lines := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		loc := d.SourcePath
		if d.Span != nil && d.Span.StartLine > 0 {
			if loc != "" {
				loc += ":"
			}
			loc += fmt.Sprintf("%d:%d", d.Span.StartLine, d.Span.StartColumn)
		}
		if loc != "" {
			lines = append(lines, loc+": "+d.Message)
			continue
		}
		lines = append(lines, d.Message)
	}
	return strings.Join(lines, "\n")
}

// ValidateAsyncPhase0 rejects async constructs that would currently compile by
// silently erasing semantics.
func ValidateAsyncPhase0(mod *Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	v := asyncValidator{sourcePath: mod.SourcePath}
	for _, d := range mod.Declarations {
		v.walkDecl(d, walkContext{})
	}
	return v.diags
}

// AsyncPhase0Error returns a DiagnosticError when unsupported async constructs exist.
func AsyncPhase0Error(mod *Module) error {
	diags := ValidateAsyncPhase0(mod)
	if len(diags) == 0 {
		return nil
	}
	return &DiagnosticError{Diagnostics: diags}
}

type walkContext struct {
	inProtected bool
}

type asyncValidator struct {
	sourcePath string
	diags      []Diagnostic
}

func (v *asyncValidator) add(span *SourceSpan, message string) {
	v.diags = append(v.diags, Diagnostic{
		SourcePath: v.sourcePath,
		Span:       span,
		Message:    message,
	})
}

func (v *asyncValidator) walkDecl(d Decl, ctx walkContext) {
	switch d := d.(type) {
	case *FuncDecl:
		if d.IsAsync {
			name := ""
			if d.Symbol != nil {
				name = d.Symbol.OriginalName
			}
			switch name {
			case "main", "init":
				v.add(d.Span, fmt.Sprintf("async %s is not supported yet", name))
			default:
				v.add(d.Span, "async function declarations are not implemented yet")
			}
		}
		v.walkParams(d.Params, ctx)
		v.walkBlock(d.Body, ctx)
	case *VarDecl:
		for _, decl := range d.Declarators {
			v.walkPattern(decl.Pattern, ctx)
			v.walkExpr(decl.Init, ctx)
		}
	case *ClassDecl:
		v.walkExpr(d.Parent, ctx)
		if d.Constructor != nil {
			v.walkParams(d.Constructor.Params, ctx)
			v.walkBlock(d.Constructor.Body, ctx)
		}
		for _, m := range d.Methods {
			if m.IsAsync {
				v.add(m.Span, "async class methods are not implemented yet")
			}
			v.walkParams(m.Params, ctx)
			v.walkBlock(m.Body, ctx)
		}
		for _, p := range d.Properties {
			v.walkExpr(p.Value, ctx)
			v.walkExpr(p.Computed, ctx)
		}
		for _, expr := range d.StaticInits {
			v.walkExpr(expr, ctx)
		}
	case *ExportDecl:
		if d.Decl != nil {
			v.walkDecl(d.Decl, ctx)
		}
	case *TopLevelStmt:
		v.walkStmt(d.Stmt, ctx)
	}
}

func (v *asyncValidator) walkBlock(b *BlockStmt, ctx walkContext) {
	if b == nil {
		return
	}
	for _, st := range b.Stmts {
		v.walkStmt(st, ctx)
	}
}

func (v *asyncValidator) walkParams(params []*Param, ctx walkContext) {
	for _, p := range params {
		if p == nil {
			continue
		}
		v.walkPattern(p.Pattern, ctx)
		v.walkExpr(p.Default, ctx)
	}
}

func (v *asyncValidator) walkPattern(p Pattern, ctx walkContext) {
	switch p := p.(type) {
	case *ObjectPattern:
		for _, prop := range p.Properties {
			v.walkPattern(prop.Pattern, ctx)
			v.walkExpr(prop.Default, ctx)
		}
	case *ArrayPattern:
		for _, elem := range p.Elements {
			if elem == nil {
				continue
			}
			v.walkPattern(elem.Pattern, ctx)
			v.walkExpr(elem.Default, ctx)
		}
	}
}

func (v *asyncValidator) walkStmt(s Stmt, ctx walkContext) {
	switch s := s.(type) {
	case *BlockStmt:
		v.walkBlock(s, ctx)
	case *ExprStmt:
		v.walkExpr(s.Expr, ctx)
	case *ReturnStmt:
		v.walkExpr(s.Value, ctx)
	case *IfStmt:
		v.walkExpr(s.Cond, ctx)
		v.walkBlock(s.Then, ctx)
		v.walkStmt(s.Else, ctx)
	case *ForStmt:
		v.walkStmt(s.Init, ctx)
		v.walkExpr(s.Cond, ctx)
		v.walkExpr(s.Post, ctx)
		v.walkBlock(s.Body, ctx)
	case *ForInStmt:
		v.walkExpr(s.Value, ctx)
		v.walkBlock(s.Body, ctx)
	case *ForOfStmt:
		v.walkExpr(s.Value, ctx)
		v.walkBlock(s.Body, ctx)
	case *WhileStmt:
		v.walkExpr(s.Cond, ctx)
		v.walkBlock(s.Body, ctx)
	case *DoWhileStmt:
		v.walkBlock(s.Body, ctx)
		v.walkExpr(s.Cond, ctx)
	case *SwitchStmt:
		v.walkExpr(s.Tag, ctx)
		for _, c := range s.Cases {
			v.walkExpr(c.Value, ctx)
			for _, st := range c.Body {
				v.walkStmt(st, ctx)
			}
		}
	case *TryCatchStmt:
		protected := walkContext{inProtected: true}
		v.walkBlock(s.Try, protected)
		if s.Catch != nil {
			v.walkBlock(s.Catch.Body, protected)
		}
		v.walkBlock(s.Finally, protected)
	case *ThrowStmt:
		v.walkExpr(s.Value, ctx)
	case *LabeledStmt:
		v.walkStmt(s.Stmt, ctx)
	case *VarDecl:
		v.walkDecl(s, ctx)
	}
}

func (v *asyncValidator) walkExpr(e Expr, ctx walkContext) {
	switch e := e.(type) {
	case *AwaitExpr:
		if ctx.inProtected {
			v.add(e.Span, "await inside try/catch/finally is not implemented yet")
		} else {
			v.add(e.Span, "await expressions are not implemented yet")
		}
		v.walkExpr(e.Value, ctx)
	case *ArrowFunc:
		if e.IsAsync {
			v.add(e.Span, "async arrow functions are not implemented yet")
		}
		v.walkParams(e.Params, ctx)
		v.walkBlock(e.Body, ctx)
		v.walkExpr(e.ExprBody, ctx)
	case *FuncExpr:
		if e.IsAsync {
			v.add(e.Span, "async function expressions are not implemented yet")
		}
		v.walkParams(e.Params, ctx)
		v.walkBlock(e.Body, ctx)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			v.walkExpr(el, ctx)
		}
	case *ObjectLiteral:
		for _, prop := range e.Properties {
			v.walkExpr(prop.Key, ctx)
			if prop.Method {
				if fn, ok := prop.Value.(*ArrowFunc); ok {
					if fn.IsAsync {
						v.add(fn.Span, "async object methods are not implemented yet")
					}
					v.walkParams(fn.Params, ctx)
					v.walkBlock(fn.Body, ctx)
					v.walkExpr(fn.ExprBody, ctx)
					continue
				}
			}
			v.walkExpr(prop.Value, ctx)
		}
	case *BinaryExpr:
		v.walkExpr(e.Left, ctx)
		v.walkExpr(e.Right, ctx)
	case *UnaryExpr:
		v.walkExpr(e.Operand, ctx)
	case *UpdateExpr:
		v.walkExpr(e.Operand, ctx)
	case *AssignExpr:
		v.walkExpr(e.Right, ctx)
		v.walkExpr(e.Left, ctx)
	case *CallExpr:
		v.walkExpr(e.Func, ctx)
		for _, arg := range e.Args {
			v.walkExpr(arg, ctx)
		}
	case *NewExpr:
		v.walkExpr(e.Callee, ctx)
		for _, arg := range e.Args {
			v.walkExpr(arg, ctx)
		}
	case *ClassExpr:
		v.walkExpr(e.Parent, ctx)
		if e.Constructor != nil {
			v.walkParams(e.Constructor.Params, ctx)
			v.walkBlock(e.Constructor.Body, ctx)
		}
		for _, m := range e.Methods {
			if m.IsAsync {
				v.add(m.Span, "async class methods are not implemented yet")
			}
			v.walkParams(m.Params, ctx)
			v.walkBlock(m.Body, ctx)
		}
		for _, p := range e.Properties {
			v.walkExpr(p.Value, ctx)
			v.walkExpr(p.Computed, ctx)
		}
		for _, expr := range e.StaticInits {
			v.walkExpr(expr, ctx)
		}
	case *MemberExpr:
		v.walkExpr(e.Object, ctx)
	case *ComputedMemberExpr:
		v.walkExpr(e.Object, ctx)
		v.walkExpr(e.Property, ctx)
	case *TernaryExpr:
		v.walkExpr(e.Cond, ctx)
		v.walkExpr(e.Then, ctx)
		v.walkExpr(e.Else, ctx)
	case *SpreadExpr:
		v.walkExpr(e.Value, ctx)
	case *SequenceExpr:
		for _, ex := range e.Exprs {
			v.walkExpr(ex, ctx)
		}
	case *YieldExpr:
		v.walkExpr(e.Value, ctx)
	case *TypeAssertExpr:
		v.walkExpr(e.Expr, ctx)
	case *NonNullExpr:
		v.walkExpr(e.Expr, ctx)
	case *ParenExpr:
		v.walkExpr(e.Expr, ctx)
	case *TaggedTemplateLiteral:
		v.walkExpr(e.Tag, ctx)
		for _, part := range e.Template.Parts {
			v.walkExpr(part, ctx)
		}
	case *TemplateLiteral:
		for _, part := range e.Parts {
			v.walkExpr(part, ctx)
		}
	}
}
