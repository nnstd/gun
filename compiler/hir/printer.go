package hir

import (
	"fmt"
	"io"
	"strings"

	"github.com/nnstd/gun/compiler/symbol"
)

// Print writes a human-readable representation of the HIR module to w.
func Print(w io.Writer, mod *Module) {
	p := &printer{w: w}
	p.printModule(mod)
}

// Sprint returns a human-readable string representation of the HIR module.
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
	p.write("Module %q", mod.Package)
	p.in()
	for _, imp := range mod.Imports {
		p.printDecl(imp)
	}
	for _, d := range mod.Declarations {
		p.printDecl(d)
	}
	p.out()
}

func (p *printer) printDecl(d Decl) {
	switch d := d.(type) {
	case *ImportDecl:
		p.printImport(d)
	case *ExportDecl:
		p.printExport(d)
	case *FuncDecl:
		p.printFuncDecl(d)
	case *VarDecl:
		p.printVarDecl(d)
	case *ClassDecl:
		p.printClassDecl(d)
	case *EnumDecl:
		p.printEnumDecl(d)
	case *InterfaceDecl:
		p.printInterfaceDecl(d)
	case *TypeAliasDecl:
		p.printTypeAliasDecl(d)
	default:
		p.write("(unknown decl %T)", d)
	}
}

func (p *printer) printImport(d *ImportDecl) {
	var parts []string
	if d.Default != nil {
		parts = append(parts, d.Default.LocalName)
	}
	if d.Namespace != nil {
		parts = append(parts, "* as "+d.Namespace.LocalName)
	}
	for _, n := range d.Named {
		if n.LocalName != n.OriginalName {
			parts = append(parts, n.OriginalName+" as "+n.LocalName)
		} else {
			parts = append(parts, n.LocalName)
		}
	}
	typeOnly := ""
	if d.TypeOnly {
		typeOnly = " (type-only)"
	}
	p.write("Import { %s } from %q%s", strings.Join(parts, ", "), d.ModulePath, typeOnly)
}

func (p *printer) printExport(d *ExportDecl) {
	if d.IsDefault {
		p.write("ExportDefault")
	} else if len(d.Names) > 0 {
		names := make([]string, len(d.Names))
		for i, n := range d.Names {
			if n.LocalName != n.ExportedName {
				names[i] = n.LocalName + " as " + n.ExportedName
			} else {
				names[i] = n.LocalName
			}
		}
		p.write("Export { %s }", strings.Join(names, ", "))
	} else {
		p.write("Export")
	}
	if d.FromModule != "" {
		p.in()
		p.write("from %q", d.FromModule)
		p.out()
	}
	if d.Decl != nil {
		p.in()
		p.printDecl(d.Decl)
		p.out()
	}
}

func (p *printer) printFuncDecl(d *FuncDecl) {
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	async := ""
	if d.IsAsync {
		async = "async "
	}
	name := symName(d.Symbol)
	p.write("%sFunction %s(%s)%s", async, name, p.paramsStr(d.Params), exported)
	if d.Body != nil {
		p.in()
		p.printBlock(d.Body)
		p.out()
	}
}

func (p *printer) printVarDecl(d *VarDecl) {
	kind := "let"
	if d.Kind == VarConst {
		kind = "const"
	} else if d.Kind == VarVar {
		kind = "var"
	}
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	for _, decl := range d.Declarators {
		name := "(pattern)"
		if decl.Symbol != nil {
			name = symName(decl.Symbol)
		}
		if decl.Init != nil {
			p.write("%s %s = %s%s", kind, name, p.exprStr(decl.Init), exported)
		} else {
			p.write("%s %s%s", kind, name, exported)
		}
	}
}

func (p *printer) printClassDecl(d *ClassDecl) {
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	parent := ""
	if d.Parent != nil {
		parent = " extends " + p.exprStr(d.Parent)
	}
	p.write("Class %s%s%s", symName(d.Symbol), parent, exported)
	p.in()
	if d.Constructor != nil {
		p.write("Constructor(%s)", p.paramsStr(d.Constructor.Params))
		if d.Constructor.Body != nil {
			p.in()
			p.printBlock(d.Constructor.Body)
			p.out()
		}
	}
	for _, m := range d.Methods {
		static := ""
		if m.IsStatic {
			static = "static "
		}
		p.write("%sMethod %s(%s)", static, m.Name, p.paramsStr(m.Params))
		if m.Body != nil {
			p.in()
			p.printBlock(m.Body)
			p.out()
		}
	}
	for _, prop := range d.Properties {
		static := ""
		if prop.IsStatic {
			static = "static "
		}
		if prop.Value != nil {
			p.write("%sProperty %s = %s", static, prop.Name, p.exprStr(prop.Value))
		} else {
			p.write("%sProperty %s", static, prop.Name)
		}
	}
	p.out()
}

func (p *printer) printEnumDecl(d *EnumDecl) {
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	p.write("Enum %s%s", symName(d.Symbol), exported)
	p.in()
	for _, m := range d.Members {
		if m.Value != nil {
			p.write("%s = %s", m.Name, p.exprStr(m.Value))
		} else {
			p.write("%s", m.Name)
		}
	}
	p.out()
}

func (p *printer) printInterfaceDecl(d *InterfaceDecl) {
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	p.write("Interface %s%s", symName(d.Symbol), exported)
	p.in()
	for _, m := range d.Members {
		if m.IsMethod {
			p.write("method %s(%d params)", m.Name, m.ParamCount)
		} else {
			p.write("prop %s: %s", m.Name, m.Type)
		}
	}
	p.out()
}

func (p *printer) printTypeAliasDecl(d *TypeAliasDecl) {
	exported := ""
	if d.Exported {
		exported = " (exported)"
	}
	p.write("TypeAlias %s = %s%s", symName(d.Symbol), d.Type, exported)
}

func (p *printer) printStmt(s Stmt) {
	switch s := s.(type) {
	case *BlockStmt:
		p.printBlock(s)
	case *ExprStmt:
		p.write("Expr %s", p.exprStr(s.Expr))
	case *ReturnStmt:
		if s.Value != nil {
			p.write("Return %s", p.exprStr(s.Value))
		} else {
			p.write("Return")
		}
	case *IfStmt:
		p.write("If %s", p.exprStr(s.Cond))
		p.in()
		p.write("Then:")
		p.in()
		p.printBlock(s.Then)
		p.out()
		if s.Else != nil {
			p.write("Else:")
			p.in()
			p.printStmt(s.Else)
			p.out()
		}
		p.out()
	case *ForStmt:
		cond := "(none)"
		if s.Cond != nil {
			cond = p.exprStr(s.Cond)
		}
		p.write("For cond=%s", cond)
		if s.Body != nil {
			p.in()
			p.printBlock(s.Body)
			p.out()
		}
	case *ForInStmt:
		p.write("ForIn %s in %s", symName(s.Key), p.exprStr(s.Value))
		if s.Body != nil {
			p.in()
			p.printBlock(s.Body)
			p.out()
		}
	case *ForOfStmt:
		name := "(pattern)"
		if s.Elem != nil {
			name = symName(s.Elem)
		}
		p.write("ForOf %s of %s", name, p.exprStr(s.Value))
		if s.Body != nil {
			p.in()
			p.printBlock(s.Body)
			p.out()
		}
	case *WhileStmt:
		p.write("While %s", p.exprStr(s.Cond))
		if s.Body != nil {
			p.in()
			p.printBlock(s.Body)
			p.out()
		}
	case *DoWhileStmt:
		p.write("DoWhile %s", p.exprStr(s.Cond))
		if s.Body != nil {
			p.in()
			p.printBlock(s.Body)
			p.out()
		}
	case *SwitchStmt:
		p.write("Switch %s", p.exprStr(s.Tag))
		p.in()
		for _, c := range s.Cases {
			if c.Value != nil {
				p.write("Case %s:", p.exprStr(c.Value))
			} else {
				p.write("Default:")
			}
			p.in()
			for _, st := range c.Body {
				p.printStmt(st)
			}
			p.out()
		}
		p.out()
	case *TryCatchStmt:
		p.write("Try")
		p.in()
		p.printBlock(s.Try)
		p.out()
		if s.Catch != nil {
			param := "(none)"
			if s.Catch.Param != nil {
				param = symName(s.Catch.Param)
			}
			p.write("Catch %s", param)
			p.in()
			p.printBlock(s.Catch.Body)
			p.out()
		}
		if s.Finally != nil {
			p.write("Finally")
			p.in()
			p.printBlock(s.Finally)
			p.out()
		}
	case *ThrowStmt:
		p.write("Throw %s", p.exprStr(s.Value))
	case *BreakStmt:
		if s.Label != "" {
			p.write("Break %s", s.Label)
		} else {
			p.write("Break")
		}
	case *ContinueStmt:
		if s.Label != "" {
			p.write("Continue %s", s.Label)
		} else {
			p.write("Continue")
		}
	case *LabeledStmt:
		p.write("Label %s:", s.Label)
		p.in()
		p.printStmt(s.Stmt)
		p.out()
	case *EmptyStmt:
		p.write("Empty")
	case *VarDecl:
		p.printVarDecl(s)
	default:
		p.write("(unknown stmt %T)", s)
	}
}

func (p *printer) printBlock(b *BlockStmt) {
	if b == nil {
		p.write("{}")
		return
	}
	for _, s := range b.Stmts {
		p.printStmt(s)
	}
}

// exprStr returns a compact one-line string for an expression.
func (p *printer) exprStr(e Expr) string {
	if e == nil {
		return "(nil)"
	}
	switch e := e.(type) {
	case *Identifier:
		if e.Sym != nil {
			return fmt.Sprintf("%s#%d", e.Sym.OriginalName, e.Sym.ID)
		}
		return e.Name
	case *Literal:
		return e.Value
	case *TemplateLiteral:
		return "`...`"
	case *ArrayLiteral:
		return fmt.Sprintf("[%d elems]", len(e.Elements))
	case *ObjectLiteral:
		return fmt.Sprintf("{%d props}", len(e.Properties))
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", p.exprStr(e.Left), binaryOpStr(e.Op), p.exprStr(e.Right))
	case *UnaryExpr:
		return fmt.Sprintf("(%s%s)", unaryOpStr(e.Op), p.exprStr(e.Operand))
	case *UpdateExpr:
		op := "++"
		if e.Op == OpDec {
			op = "--"
		}
		if e.Prefix {
			return fmt.Sprintf("(%s%s)", op, p.exprStr(e.Operand))
		}
		return fmt.Sprintf("(%s%s)", p.exprStr(e.Operand), op)
	case *AssignExpr:
		return fmt.Sprintf("(%s %s %s)", p.exprStr(e.Left), assignOpStr(e.Op), p.exprStr(e.Right))
	case *CallExpr:
		args := make([]string, len(e.Args))
		for i, a := range e.Args {
			args[i] = p.exprStr(a)
		}
		return fmt.Sprintf("%s(%s)", p.exprStr(e.Func), strings.Join(args, ", "))
	case *NewExpr:
		return fmt.Sprintf("new %s(...)", p.exprStr(e.Callee))
	case *MemberExpr:
		return fmt.Sprintf("%s.%s", p.exprStr(e.Object), e.Property)
	case *ComputedMemberExpr:
		return fmt.Sprintf("%s[%s]", p.exprStr(e.Object), p.exprStr(e.Property))
	case *TernaryExpr:
		return fmt.Sprintf("(%s ? %s : %s)", p.exprStr(e.Cond), p.exprStr(e.Then), p.exprStr(e.Else))
	case *ArrowFunc:
		return fmt.Sprintf("(%s) => ...", p.paramsStr(e.Params))
	case *FuncExpr:
		return fmt.Sprintf("function %s(%s)", e.Name, p.paramsStr(e.Params))
	case *SpreadExpr:
		return fmt.Sprintf("...%s", p.exprStr(e.Value))
	case *AwaitExpr:
		return fmt.Sprintf("await %s", p.exprStr(e.Value))
	case *ThisExpr:
		return "this"
	case *SuperExpr:
		return "super"
	case *ParenExpr:
		return fmt.Sprintf("(%s)", p.exprStr(e.Expr))
	case *SequenceExpr:
		parts := make([]string, len(e.Exprs))
		for i, ex := range e.Exprs {
			parts[i] = p.exprStr(ex)
		}
		return strings.Join(parts, ", ")
	case *TypeAssertExpr:
		return fmt.Sprintf("(%s as %s)", p.exprStr(e.Expr), e.Type)
	case *NonNullExpr:
		return fmt.Sprintf("%s!", p.exprStr(e.Expr))
	case *MetaPropertyExpr:
		return fmt.Sprintf("%s.%s", e.Meta, e.Property)
	case *TaggedTemplateLiteral:
		return fmt.Sprintf("%s`...`", p.exprStr(e.Tag))
	case *YieldExpr:
		if e.Delegate {
			return fmt.Sprintf("yield* %s", p.exprStr(e.Value))
		}
		return fmt.Sprintf("yield %s", p.exprStr(e.Value))
	default:
		return fmt.Sprintf("(?%T)", e)
	}
}

func (p *printer) paramsStr(params []*Param) string {
	parts := make([]string, len(params))
	for i, param := range params {
		name := "(pattern)"
		if param.Symbol != nil {
			name = symName(param.Symbol)
		}
		if param.Rest {
			name = "..." + name
		}
		parts[i] = name
	}
	return strings.Join(parts, ", ")
}

func symName(sym *symbol.Symbol) string {
	if sym == nil {
		return "(nil)"
	}
	return fmt.Sprintf("%s#%d", sym.OriginalName, sym.ID)
}

func binaryOpStr(op BinaryOp) string {
	switch op {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpExp:
		return "**"
	case OpEq:
		return "==="
	case OpNEq:
		return "!=="
	case OpEqLoose:
		return "=="
	case OpNEqLoose:
		return "!="
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLtE:
		return "<="
	case OpGtE:
		return ">="
	case OpAnd:
		return "&&"
	case OpOr:
		return "||"
	case OpNullish:
		return "??"
	case OpBitAnd:
		return "&"
	case OpBitOr:
		return "|"
	case OpBitXor:
		return "^"
	case OpShl:
		return "<<"
	case OpShr:
		return ">>"
	case OpUShr:
		return ">>>"
	case OpIn:
		return "in"
	case OpInstanceof:
		return "instanceof"
	default:
		return "?op"
	}
}

func unaryOpStr(op UnaryOp) string {
	switch op {
	case OpNot:
		return "!"
	case OpNeg:
		return "-"
	case OpPos:
		return "+"
	case OpBitNot:
		return "~"
	case OpTypeof:
		return "typeof "
	case OpVoid:
		return "void "
	case OpDelete:
		return "delete "
	default:
		return "?op"
	}
}

func assignOpStr(op AssignOp) string {
	switch op {
	case OpAssign:
		return "="
	case OpAddAssign:
		return "+="
	case OpSubAssign:
		return "-="
	case OpMulAssign:
		return "*="
	case OpDivAssign:
		return "/="
	case OpModAssign:
		return "%="
	case OpBitAndAssign:
		return "&="
	case OpBitOrAssign:
		return "|="
	case OpBitXorAssign:
		return "^="
	case OpShlAssign:
		return "<<="
	case OpShrAssign:
		return ">>="
	case OpUShrAssign:
		return ">>>="
	case OpNullishAssign:
		return "??="
	case OpAndAssign:
		return "&&="
	case OpOrAssign:
		return "||="
	case OpExpAssign:
		return "**="
	default:
		return "?="
	}
}
