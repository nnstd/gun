package mir

import (
	"fmt"
	"io"
	"strings"
)

// Print writes a human-readable representation of the MIR module to w.
func Print(w io.Writer, mod *Module) {
	p := &printer{w: w}
	p.printModule(mod)
}

// Sprint returns a human-readable string representation of the MIR module.
func Sprint(mod *Module) string {
	var b strings.Builder
	Print(&b, mod)
	return b.String()
}

type printer struct {
	w      io.Writer
	indent int
}

func (p *printer) write(format string, args ...any) {
	fmt.Fprintf(p.w, "%s", strings.Repeat("  ", p.indent))
	fmt.Fprintf(p.w, format, args...)
	fmt.Fprintln(p.w)
}

func (p *printer) in()  { p.indent++ }
func (p *printer) out() { p.indent-- }

func (p *printer) printModule(mod *Module) {
	p.write("MIR Module %q", mod.Package)
	p.in()
	for _, imp := range mod.Imports {
		if imp.Alias != "" {
			p.write("import %s %q", imp.Alias, imp.Path)
		} else {
			p.write("import %q", imp.Path)
		}
	}
	for _, g := range mod.Globals {
		name := "(nil)"
		if g.Symbol != nil {
			name = g.Symbol.OriginalName
		}
		if g.Init != nil {
			p.write("global %s = %s", name, p.exprStr(g.Init))
		} else {
			p.write("global %s", name)
		}
	}
	for _, fn := range mod.Functions {
		p.printFunc(fn)
	}
	p.out()
}

func (p *printer) printFunc(fn *Function) {
	name := "(anon)"
	if fn.Symbol != nil {
		name = fn.Symbol.OriginalName
	}
	var params []string
	for _, param := range fn.Params {
		pname := "?"
		if param.Symbol != nil {
			pname = param.Symbol.OriginalName
		}
		if param.Rest {
			pname = "..." + pname
		}
		params = append(params, pname)
	}

	flags := ""
	if fn.Exported {
		flags += " (exported)"
	}
	if fn.IsMain {
		flags += " (main)"
	}

	p.write("func %s(%s)%s {", name, strings.Join(params, ", "), flags)
	p.in()
	for _, b := range fn.Blocks {
		p.printBlock(b)
	}
	p.out()
	p.write("}")
}

func (p *printer) printBlock(b *BasicBlock) {
	preds := make([]string, len(b.Preds))
	for i, pred := range b.Preds {
		preds[i] = fmt.Sprintf("bb%d", pred.ID)
	}
	predStr := ""
	if len(preds) > 0 {
		predStr = fmt.Sprintf(" <- [%s]", strings.Join(preds, ", "))
	}
	p.write("bb%d:%s", b.ID, predStr)
	p.in()
	for _, s := range b.Stmts {
		p.printStmt(s)
	}
	if b.Term != nil {
		p.printTerm(b.Term)
	}
	p.out()
}

func (p *printer) printStmt(s Stmt) {
	switch s := s.(type) {
	case *AssignStmt:
		name := "?"
		if s.Target != nil {
			name = s.Target.OriginalName
		}
		p.write("%s = %s", name, p.exprStr(s.Value))
	case *StoreStmt:
		p.write("%s[%s] = %s", p.exprStr(s.Object), p.exprStr(s.Key), p.exprStr(s.Value))
	case *ExprStmt:
		p.write("expr %s", p.exprStr(s.Expr))
	case *DeclStmt:
		name := "?"
		if s.Symbol != nil {
			name = s.Symbol.OriginalName
		}
		if s.Value != nil {
			p.write("decl %s = %s", name, p.exprStr(s.Value))
		} else {
			p.write("decl %s", name)
		}
	case *DeferStmt:
		p.write("defer %s", p.exprStr(s.Call))
	default:
		p.write("(unknown stmt)")
	}
}

func (p *printer) printTerm(t Terminator) {
	switch t := t.(type) {
	case *JumpTerm:
		target := "?"
		if t.Target != nil {
			target = fmt.Sprintf("bb%d", t.Target.ID)
		}
		p.write("-> %s", target)
	case *BranchTerm:
		trueID, falseID := "?", "?"
		if t.True != nil {
			trueID = fmt.Sprintf("bb%d", t.True.ID)
		}
		if t.False != nil {
			falseID = fmt.Sprintf("bb%d", t.False.ID)
		}
		p.write("branch %s ? %s : %s", p.exprStr(t.Cond), trueID, falseID)
	case *ReturnTerm:
		if t.Value != nil {
			p.write("return %s", p.exprStr(t.Value))
		} else {
			p.write("return")
		}
	case *SwitchTerm:
		p.write("switch %s {", p.exprStr(t.Tag))
		p.in()
		for _, c := range t.Cases {
			target := "?"
			if c.Target != nil {
				target = fmt.Sprintf("bb%d", c.Target.ID)
			}
			p.write("case %s -> %s", p.exprStr(c.Value), target)
		}
		if t.Default != nil {
			p.write("default -> bb%d", t.Default.ID)
		}
		p.out()
		p.write("}")
	case *PanicTerm:
		p.write("panic %s", p.exprStr(t.Value))
	default:
		p.write("(unknown term)")
	}
}

func (p *printer) exprStr(e Expr) string {
	if e == nil {
		return "(nil)"
	}
	switch e := e.(type) {
	case *IdentExpr:
		if e.Symbol != nil {
			return e.Symbol.OriginalName
		}
		return e.Name
	case *LitExpr:
		if e.Kind == LitString {
			return fmt.Sprintf("%q", e.Value)
		}
		return e.Value
	case *BinExpr:
		return fmt.Sprintf("(%s %s %s)", p.exprStr(e.Left), binOpStr(e.Op), p.exprStr(e.Right))
	case *UnaryExpr:
		return fmt.Sprintf("(%s%s)", unaryOpStr(e.Op), p.exprStr(e.Operand))
	case *CallExpr:
		args := make([]string, len(e.Args))
		for i, a := range e.Args {
			args[i] = p.exprStr(a)
		}
		return fmt.Sprintf("%s(%s)", p.exprStr(e.Func), strings.Join(args, ", "))
	case *NewCallExpr:
		return fmt.Sprintf("new %s(...)", p.exprStr(e.Callee))
	case *GetExpr:
		return fmt.Sprintf("%s.%s", p.exprStr(e.Object), p.exprStr(e.Key))
	case *IndexExpr:
		return fmt.Sprintf("%s[%s]", p.exprStr(e.Object), p.exprStr(e.Index))
	case *FuncExpr:
		return "(func)"
	case *ArrayExpr:
		return fmt.Sprintf("[%d elems]", len(e.Elements))
	case *ObjectExpr:
		return fmt.Sprintf("{%d props}", len(e.Keys))
	case *SpreadExpr:
		return fmt.Sprintf("...%s", p.exprStr(e.Value))
	case *TemplateExpr:
		return "`...`"
	case *ThisExpr:
		return "this"
	case *NilExpr:
		return "nil"
	default:
		return fmt.Sprintf("(?%T)", e)
	}
}

func binOpStr(op BinOp) string {
	ops := [...]string{
		"+", "-", "*", "/", "%", "**",
		"===", "!==", "==", "!=",
		"<", ">", "<=", ">=",
		"&&", "||", "??",
		"&", "|", "^", "<<", ">>", ">>>",
		"in", "instanceof",
	}
	if int(op) < len(ops) {
		return ops[op]
	}
	return "?op"
}

func unaryOpStr(op UnaryOp) string {
	ops := [...]string{"!", "-", "+", "~", "typeof ", "void "}
	if int(op) < len(ops) {
		return ops[op]
	}
	return "?op"
}
