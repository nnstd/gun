package backend

import (
	"go/ast"
	"go/token"
	"path"

	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// Lowerer converts an HIR Module into a go/ast.File.
type Lowerer struct {
	symtab  *symbol.Table
	ctx     *context.TranspilerContext
	imports map[string]string // Go import path → alias
	decls   []ast.Decl
}

// Lower converts an HIR module to a Go AST file.
func Lower(mod *hir.Module, ctx *context.TranspilerContext) *ast.File {
	l := &Lowerer{
		symtab:  mod.SymbolTable,
		ctx:     ctx,
		imports: make(map[string]string),
	}

	// Lower all declarations
	for _, d := range mod.Imports {
		l.lowerImportDecl(d)
	}
	for _, d := range mod.Declarations {
		l.lowerDecl(d)
	}

	file := &ast.File{
		Name:  goIdent(mod.Package),
		Decls: l.decls,
	}

	// Assemble import declarations
	if len(l.imports) > 0 {
		var specs []ast.Spec
		for pkg, alias := range l.imports {
			specs = append(specs, importSpecAlias(pkg, alias))
		}
		if len(specs) > 0 {
			importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: specs}
			if len(specs) > 1 {
				importDecl.Lparen = 1
			}
			file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
		}
	}

	return file
}

func (l *Lowerer) addImport(pkg string) {
	if _, ok := l.imports[pkg]; !ok {
		l.imports[pkg] = ""
	}
}

func (l *Lowerer) addAliasedImport(pkg, alias string) {
	l.imports[pkg] = alias
}

func (l *Lowerer) jsvalueImport() {
	l.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
}

func (l *Lowerer) emitName(sym *symbol.Symbol) string {
	if sym == nil {
		return "_"
	}
	return l.symtab.EmitName(sym)
}

// --------------------------------------------------------------------
// Declarations
// --------------------------------------------------------------------

func (l *Lowerer) lowerDecl(d hir.Decl) {
	switch d := d.(type) {
	case *hir.FuncDecl:
		l.lowerFuncDecl(d)
	case *hir.VarDecl:
		l.lowerVarDecl(d)
	case *hir.ClassDecl:
		l.lowerClassDecl(d)
	case *hir.EnumDecl:
		l.lowerEnumDecl(d)
	case *hir.InterfaceDecl:
		l.lowerInterfaceDecl(d)
	case *hir.TypeAliasDecl:
		l.lowerTypeAliasDecl(d)
	case *hir.ExportDecl:
		l.lowerExportDecl(d)
	case *hir.ImportDecl:
		l.lowerImportDecl(d)
	case *hir.TopLevelStmt:
		l.lowerTopLevelStmt(d)
	}
}

func (l *Lowerer) lowerFuncDecl(d *hir.FuncDecl) {
	name := l.emitName(d.Symbol)

	// main and init stay as Go func declarations
	if name == "main" || name == "init" {
		body := l.lowerBlock(d.Body)
		fd := funcDecl(name, fieldList(), nil, body)
		l.decls = append(l.decls, fd)
		return
	}

	// All other functions become jsvalue.NewFunction vars
	l.jsvalueImport()
	body := l.lowerFuncBody(d.Params, d.Body)
	fnLit := l.wrapAsJSValueFunc(d.Params, body)
	l.decls = append(l.decls, varDecl(name, nil, callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), fnLit)))
}

func (l *Lowerer) lowerVarDecl(d *hir.VarDecl) {
	for _, decl := range d.Declarators {
		if decl.Symbol == nil {
			continue
		}
		name := l.emitName(decl.Symbol)
		var value ast.Expr
		if decl.Init != nil {
			value = l.lowerExpr(decl.Init)
			value = jsvalueWrapLit(value)
		}
		l.decls = append(l.decls, varDecl(name, nil, value))
	}
}

func (l *Lowerer) lowerClassDecl(d *hir.ClassDecl) {
	l.jsvalueImport()
	name := l.emitName(d.Symbol)

	// Build constructor function
	var ctorBody *ast.BlockStmt
	if d.Constructor != nil {
		ctorBody = l.lowerFuncBody(d.Constructor.Params, d.Constructor.Body)
	} else {
		ctorBody = blockStmt()
	}
	ctorLit := l.wrapAsJSValueFunc(nil, ctorBody)

	// jsvalue.NewClass(ctorFn, parent?)
	args := []ast.Expr{ctorLit}
	if d.Parent != nil {
		args = append(args, l.lowerExpr(d.Parent))
	}
	newClassCall := callExpr(selectorExpr(goIdent("jsvalue"), "NewClass"), args...)
	l.decls = append(l.decls, varDecl(name, nil, newClassCall))

	// Methods: ClassName.Get("prototype").Set("method", jsvalue.NewFunction(...))
	for _, m := range d.Methods {
		methodBody := l.lowerFuncBody(m.Params, m.Body)
		methodLit := l.wrapAsJSValueFunc(m.Params, methodBody)
		methodFn := callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), methodLit)

		var setCall ast.Expr
		if m.IsStatic {
			setCall = callExpr(selectorExpr(goIdent(name), "Set"), stringLit(m.Name), methodFn)
		} else {
			proto := callExpr(selectorExpr(goIdent(name), "Get"), stringLit("prototype"))
			setCall = callExpr(selectorExpr(proto, "Set"), stringLit(m.Name), methodFn)
		}
		l.decls = append(l.decls, &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names:  []*ast.Ident{goIdent("_")},
				Values: []ast.Expr{setCall},
			}},
		})
	}
}

func (l *Lowerer) lowerEnumDecl(d *hir.EnumDecl) {
	name := l.emitName(d.Symbol)

	// Check if it's a string enum
	isString := false
	for _, m := range d.Members {
		if m.Value != nil {
			if lit, ok := m.Value.(*hir.Literal); ok && lit.Kind == hir.LitString {
				isString = true
				break
			}
		}
	}

	if isString {
		// String enum: type Name string + const block
		l.decls = append(l.decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: goIdent(name),
				Assign: 1,
				Type: goIdent("string"),
			}},
		})
		var specs []ast.Spec
		for _, m := range d.Members {
			val := stringLit(m.Name)
			if m.Value != nil {
				val = l.lowerExpr(m.Value).(*ast.BasicLit)
			}
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{goIdent(name + m.Name)},
				Values: []ast.Expr{val},
			})
		}
		l.decls = append(l.decls, &ast.GenDecl{Tok: token.CONST, Specs: specs, Lparen: 1})
	} else {
		// Numeric enum: type Name int + const with iota
		l.decls = append(l.decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: goIdent(name),
				Assign: 1,
				Type: goIdent("int"),
			}},
		})
		var specs []ast.Spec
		for i, m := range d.Members {
			vs := &ast.ValueSpec{
				Names: []*ast.Ident{goIdent(name + m.Name)},
			}
			if m.Value != nil {
				vs.Values = []ast.Expr{l.lowerExpr(m.Value)}
			} else if i == 0 {
				vs.Values = []ast.Expr{goIdent("iota")}
			}
			specs = append(specs, vs)
		}
		l.decls = append(l.decls, &ast.GenDecl{Tok: token.CONST, Specs: specs, Lparen: 1})
	}
}

func (l *Lowerer) lowerInterfaceDecl(d *hir.InterfaceDecl) {
	name := l.emitName(d.Symbol)
	l.jsvalueImport()

	hasMethods := false
	hasProps := false
	for _, m := range d.Members {
		if m.IsMethod {
			hasMethods = true
		} else {
			hasProps = true
		}
	}

	if hasMethods && !hasProps {
		// Pure method interface → Go interface
		var methods []*ast.Field
		for _, m := range d.Members {
			params := fieldList()
			for i := 0; i < m.ParamCount; i++ {
				params.List = append(params.List, goField("", jsValuePtrType()))
			}
			methods = append(methods, &ast.Field{
				Names: []*ast.Ident{goIdent(symbol.Capitalize(m.Name))},
				Type: &ast.FuncType{
					Params:  params,
					Results: fieldList(goField("", jsValuePtrType())),
				},
			})
		}
		l.decls = append(l.decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: goIdent(name),
				Type: &ast.InterfaceType{Methods: &ast.FieldList{List: methods}},
			}},
		})
	} else {
		// Mixed or props-only → Go struct
		var fields []*ast.Field
		for _, m := range d.Members {
			fields = append(fields, goField(symbol.Capitalize(m.Name), jsValuePtrType()))
		}
		l.decls = append(l.decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: goIdent(name),
				Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
			}},
		})
	}
}

func (l *Lowerer) lowerTypeAliasDecl(d *hir.TypeAliasDecl) {
	name := l.emitName(d.Symbol)
	l.jsvalueImport()
	l.decls = append(l.decls, &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Name:   goIdent(name),
			Assign: 1,
			Type:   jsValuePtrType(),
		}},
	})
}

func (l *Lowerer) lowerExportDecl(d *hir.ExportDecl) {
	if d.Decl != nil {
		// Export wraps a declaration — mark its symbol as exported and lower it
		switch inner := d.Decl.(type) {
		case *hir.FuncDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerFuncDecl(inner)
		case *hir.VarDecl:
			for _, decl := range inner.Declarators {
				if decl.Symbol != nil {
					decl.Symbol.Exported = true
				}
			}
			l.lowerVarDecl(inner)
		case *hir.ClassDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerClassDecl(inner)
		case *hir.EnumDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerEnumDecl(inner)
		case *hir.InterfaceDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerInterfaceDecl(inner)
		case *hir.TypeAliasDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerTypeAliasDecl(inner)
		default:
			l.lowerDecl(d.Decl)
		}
	}
}

func (l *Lowerer) lowerImportDecl(d *hir.ImportDecl) {
	if l.ctx == nil {
		return
	}
	// Resolve module through the context
	mod := l.ctx.LookupModule(d.ModulePath)
	if mod != nil {
		if mod.GoPkgName != "" && mod.GoPkgName != path.Base(mod.GoImportPath) {
			l.addAliasedImport(mod.GoImportPath, mod.GoPkgName)
		} else if mod.GoImportPath != "" {
			l.addImport(mod.GoImportPath)
		}
	}
}

func (l *Lowerer) lowerTopLevelStmt(d *hir.TopLevelStmt) {
	mainFn := l.getOrCreateMain()
	if gs := l.lowerStmt(d.Stmt); gs != nil {
		mainFn.Body.List = append(mainFn.Body.List, gs)
	}
}

// getOrCreateMain returns the main() function declaration, creating it if needed.
func (l *Lowerer) getOrCreateMain() *ast.FuncDecl {
	// Look for existing main func
	for _, d := range l.decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "main" {
			return fd
		}
	}
	// Create one
	fd := funcDecl("main", fieldList(), nil, blockStmt())
	l.decls = append(l.decls, fd)
	return fd
}

// --------------------------------------------------------------------
// Function helpers
// --------------------------------------------------------------------

func (l *Lowerer) lowerFuncBody(params []*hir.Param, body *hir.BlockStmt) *ast.BlockStmt {
	if body == nil {
		return blockStmt()
	}

	var stmts []ast.Stmt

	// Unpack _args into named parameters
	for i, p := range params {
		if p.Symbol == nil {
			continue
		}
		name := l.emitName(p.Symbol)
		if p.Rest {
			// Rest param: name := jsvalue.NewArray(_args[i:]...)
			l.jsvalueImport()
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{goIdent(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{
					callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"),
						&ast.SliceExpr{X: goIdent("_args"), Low: intLit(itoa(i))}),
				},
			})
		} else {
			// name := _args[i]
			idx := &ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(i))}
			stmts = append(stmts, assignDefine(
				[]ast.Expr{goIdent(name)},
				[]ast.Expr{idx},
			))

			// Default value handling
			if p.Default != nil {
				defVal := l.lowerExpr(p.Default)
				defVal = jsvalueWrapLit(defVal)
				stmts = append(stmts, &ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X:  goIdent(name),
						Op: token.EQL,
						Y:  goIdent("nil"),
					},
					Body: blockStmt(&ast.AssignStmt{
						Lhs: []ast.Expr{goIdent(name)},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{defVal},
					}),
				})
			}
		}
	}

	// Lower body statements
	for _, s := range body.Stmts {
		if gs := l.lowerStmt(s); gs != nil {
			stmts = append(stmts, gs)
		}
	}

	return &ast.BlockStmt{List: stmts}
}

func (l *Lowerer) wrapAsJSValueFunc(params []*hir.Param, body *ast.BlockStmt) *ast.FuncLit {
	l.jsvalueImport()
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: fieldList(&ast.Field{
				Names: []*ast.Ident{goIdent("_args")},
				Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
			}),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: body,
	}
}
