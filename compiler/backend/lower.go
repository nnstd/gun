package backend

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// importResolution describes how an imported symbol maps to Go.
type importResolution struct {
	goImportPath string
	goPkgName    string
	goSymbol     string
	isTranspiled bool
}

// CrossFileExport describes a symbol exported from another file in the same package.
type CrossFileExport struct {
	GoName    string
	IsJSValue bool
}

// Lowerer converts an HIR Module into a go/ast.File.
type Lowerer struct {
	symtab           *symbol.Table
	ctx              *context.TranspilerContext
	imports          map[string]string // Go import path → alias
	decls            []ast.Decl
	importedSyms     map[*symbol.Symbol]importResolution // how each imported symbol resolves to Go
	moduleName       string                              // Go module name for relative import resolution
	samePackage      bool                                // treat relative imports as same-package refs
	varTypes         map[string]string                   // variable name → module type (e.g. "hono")
	crossFileExports map[string]bool                     // Go names from other files (prevents .Get() dispatch)
	initStmts        []ast.Stmt                          // statements for init() function
	pkgName          string                              // Go package name
	currentClassName string                              // set during class constructor/method lowering
	insideFunc       int                                 // >0 when inside a function body
	insideMethod     int                                 // >0 when inside a method body (_args[0] is this)
	privateKeys      map[string]string
	syntheticCounter int
	needsBunWait     bool
}

// Lower converts an HIR module to a Go AST file.
func Lower(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool) *ast.File {
	return LowerWithExports(mod, ctx, moduleName, samePackageImports, nil)
}

// LowerWithExports converts an HIR module to a Go AST file with knowledge of
// symbols exported from other files in the same package.
func LowerWithExports(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool, crossFileExports []CrossFileExport) *ast.File {
	cfe := make(map[string]bool)
	for _, exp := range crossFileExports {
		cfe[exp.GoName] = true
	}
	l := &Lowerer{
		symtab:           mod.SymbolTable,
		ctx:              ctx,
		imports:          make(map[string]string),
		importedSyms:     make(map[*symbol.Symbol]importResolution),
		moduleName:       moduleName,
		samePackage:      samePackageImports,
		crossFileExports: cfe,
		pkgName:          mod.Package,
		varTypes:         make(map[string]string),
	}

	// Reserve cross-file export names in the symbol table so local symbols
	// get suffixed on collision instead of redeclaring.
	for name := range l.crossFileExports {
		l.symtab.ReserveNameStr(name)
	}

	// Pre-scan: collect function param counts for nil-padding callers
	l.prescan(mod)

	// Lower all declarations
	for _, d := range mod.Imports {
		l.lowerImportDecl(d)
	}
	for _, d := range mod.Declarations {
		l.lowerDecl(d)
	}

	// SWC interop: synthesize var Default = PrimaryExport when requested
	if mod.SynthesizeDefault != "" {
		l.decls = append(l.decls, varDecl("Default", nil, goIdent(mod.SynthesizeDefault)))
	}

	// Fix init cycles: split self-referencing vars into forward decl + init()
	l.decls = l.fixInitCycles(l.decls)

	// Ensure main() exists for runnable packages
	if mod.Package == "main" {
		l.getOrCreateMain()
		if l.needsBunWait {
			l.addImport("github.com/nnstd/gun/runtime/bun")
			if mainFn := l.findMainFunc(); mainFn != nil {
				mainFn.Body.List = append(mainFn.Body.List, exprStmt(callExpr(selectorExpr(goIdent("bun"), "Wait"))))
			}
		}
	}

	// Emit init() for collected setup statements (class methods, enum members, etc.)
	if len(l.initStmts) > 0 {
		l.decls = append(l.decls, funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: l.initStmts}))
	}

	file := &ast.File{
		Name:  goIdent(mod.Package),
		Decls: l.decls,
	}

	// Prune unused imports: only keep imports whose package ident appears in the code
	usedIdents := collectUsedIdents(l.decls)
	for pkg, alias := range l.imports {
		pkgIdent := alias
		if pkgIdent == "" {
			pkgIdent = path.Base(pkg)
		}
		if !usedIdents[pkgIdent] {
			delete(l.imports, pkg)
		}
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

func (l *Lowerer) findMainFunc() *ast.FuncDecl {
	for _, d := range l.decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name != nil && fd.Name.Name == "main" {
			return fd
		}
	}
	return nil
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
			// Track varTypes for module-specific dispatch (e.g. new Hono() → "hono")
			if newExpr, ok := decl.Init.(*hir.NewExpr); ok {
				if id, ok := newExpr.Callee.(*hir.Identifier); ok {
					ctorName := id.Name
					if id.Sym != nil {
						ctorName = id.Sym.OriginalName
					}
					modType := strings.ToLower(ctorName)
					l.varTypes[name] = modType
					if decl.Symbol != nil {
						decl.Symbol.ModuleType = modType
					}
				}
			}
			value = l.lowerExpr(decl.Init)
			value = jsvalueWrapLit(value)
		}
		if value == nil {
			// Uninitialized var needs a type: var x *jsvalue.JSValue
			l.jsvalueImport()
			l.decls = append(l.decls, varDecl(name, jsValuePtrType(), nil))
		} else {
			l.decls = append(l.decls, varDecl(name, nil, value))
		}
	}
}

func (l *Lowerer) lowerClassDecl(d *hir.ClassDecl) {
	l.jsvalueImport()
	name := l.emitName(d.Symbol)
	privateKeys := l.collectPrivateKeys(name, d.Properties, d.Methods)
	for _, decl := range l.lowerPrivateKeyDecls(name, privateKeys) {
		l.decls = append(l.decls, decl)
	}

	prevClassName := l.currentClassName
	prevPrivateKeys := l.privateKeys
	l.currentClassName = name
	l.privateKeys = privateKeys
	defer func() {
		l.currentClassName = prevClassName
		l.privateKeys = prevPrivateKeys
	}()

	var parentExpr ast.Expr = goIdent("nil")
	if d.Parent != nil {
		parentExpr = l.lowerExpr(d.Parent)
	}

	ctorLit := l.lowerClassConstructor(name, d.Parent != nil, d.Constructor, d.Properties, d.Methods)
	l.decls = append(l.decls, varDecl(name, nil,
		callExpr(selectorExpr(goIdent("jsvalue"), "NewClass"), ctorLit, parentExpr)))

	l.initStmts = append(l.initStmts, l.lowerClassSetups(goIdent(name), d.Properties, d.Methods, d.StaticInits)...)
}

func (l *Lowerer) collectPrivateKeys(className string, props []*hir.ClassProperty, methods []*hir.ClassMethod) map[string]string {
	keys := make(map[string]string)
	add := func(member string) {
		if member == "" {
			return
		}
		if _, ok := keys[member]; ok {
			return
		}
		keys[member] = l.nextSyntheticName(fmt.Sprintf("_private_%s_%s", symbol.Sanitize(className), symbol.Sanitize(member)))
	}
	for _, prop := range props {
		if prop.IsPrivate {
			add(prop.Name)
		}
	}
	for _, method := range methods {
		if method.IsPrivate {
			add(method.Name)
		}
	}
	return keys
}

func (l *Lowerer) lowerPrivateKeyDecls(className string, keys map[string]string) []ast.Decl {
	if len(keys) == 0 {
		return nil
	}
	l.jsvalueImport()
	var decls []ast.Decl
	for member, goName := range keys {
		desc := fmt.Sprintf("%s.#%s", className, member)
		value := callExpr(
			selectorExpr(goIdent("jsvalue"), "PropertyKey"),
			callExpr(selectorExpr(goIdent("jsvalue"), "NewSymbol"), stringLit(desc)),
		)
		decls = append(decls, varDecl(goName, nil, value))
	}
	return decls
}

func (l *Lowerer) nextSyntheticName(base string) string {
	l.syntheticCounter++
	name := fmt.Sprintf("%s_%d", base, l.syntheticCounter)
	l.symtab.ReserveNameStr(name)
	return name
}

func (l *Lowerer) lowerClassConstructor(className string, hasParent bool, ctor *hir.ClassConstructor, props []*hir.ClassProperty, methods []*hir.ClassMethod) *ast.FuncLit {
	var ctorBody *ast.BlockStmt
	if ctor != nil {
		ctorBody = l.lowerFuncBody(ctor.Params, ctor.Body)
	} else {
		ctorBody = blockStmt()
		if hasParent {
			superCall := callExpr(selectorExpr(goIdent(className), "CallSuper"), goIdent("this"), goIdent("_args"))
			superCall.Ellipsis = 1
			ctorBody.List = append(ctorBody.List, exprStmt(superCall))
		}
	}

	instanceInits := l.lowerClassInstanceInits(props, methods)
	if len(instanceInits) > 0 {
		insertAt := 0
		if hasParent && len(ctorBody.List) > 0 && isCallSuperStmt(ctorBody.List[0], className) {
			insertAt = 1
		}
		ctorBody.List = append(ctorBody.List[:insertAt], append(instanceInits, ctorBody.List[insertAt:]...)...)
	}
	ctorBody.List = append(ctorBody.List, returnStmt(goIdent("nil")))

	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: fieldList(
				goField("this", jsValuePtrType()),
				&ast.Field{
					Names: []*ast.Ident{goIdent("_args")},
					Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
				},
			),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: ctorBody,
	}
}

func isCallSuperStmt(stmt ast.Stmt, className string) bool {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "CallSuper" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == className
}

func (l *Lowerer) lowerClassInstanceInits(props []*hir.ClassProperty, methods []*hir.ClassMethod) []ast.Stmt {
	var stmts []ast.Stmt
	for _, prop := range props {
		if prop.IsStatic {
			continue
		}
		var value ast.Expr = callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
		if prop.Value != nil {
			value = jsvalueWrapLit(l.lowerExpr(prop.Value))
		}
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(goIdent("this"), "Set"), l.lowerClassMemberKey(prop.Name, prop.IsPrivate, prop.Computed), value)))
	}
	for _, method := range methods {
		if method.IsStatic || !method.IsPrivate {
			continue
		}
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(goIdent("this"), "Set"), l.lowerClassMemberKey(method.Name, true, nil), l.lowerClassMethodValue(method))))
	}
	return stmts
}

func (l *Lowerer) lowerClassSetups(classRef ast.Expr, props []*hir.ClassProperty, methods []*hir.ClassMethod, staticInits []hir.Expr) []ast.Stmt {
	var stmts []ast.Stmt
	for _, prop := range props {
		if !prop.IsStatic {
			continue
		}
		var value ast.Expr = callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
		if prop.Value != nil {
			value = jsvalueWrapLit(l.lowerExpr(prop.Value))
		}
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(classRef, "Set"), l.lowerClassMemberKey(prop.Name, prop.IsPrivate, prop.Computed), value)))
	}
	for _, expr := range staticInits {
		if lowered := l.lowerExpr(expr); lowered != nil {
			stmts = append(stmts, exprStmt(lowered))
		}
	}
	for _, method := range methods {
		if !method.IsStatic && method.IsPrivate {
			continue
		}
		if !method.IsStatic && !method.IsPrivate {
			proto := callExpr(selectorExpr(classRef, "Get"), stringLit("prototype"))
			stmts = append(stmts, exprStmt(callExpr(selectorExpr(proto, "Set"), l.lowerClassMemberKey(method.Name, false, method.Computed), l.lowerClassMethodValue(method))))
			continue
		}
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(classRef, "Set"), l.lowerClassMemberKey(method.Name, method.IsPrivate, method.Computed), l.lowerClassMethodValue(method))))
	}
	return stmts
}

func (l *Lowerer) lowerClassMethodValue(m *hir.ClassMethod) ast.Expr {
	var methodBody *ast.BlockStmt
	if !m.IsStatic {
		methodBody = l.lowerMethodBody(m.Params, m.Body)
	} else {
		methodBody = l.lowerFuncBody(m.Params, m.Body)
	}
	methodLit := l.wrapAsJSValueFunc(m.Params, methodBody)
	return callExpr(selectorExpr(
		callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), methodLit),
		"MarkAsMethod"))
}

func (l *Lowerer) lowerClassMemberKey(name string, isPrivate bool, computed hir.Expr) ast.Expr {
	if isPrivate {
		return l.privateKeyExpr(name)
	}
	if computed != nil {
		l.addImport("fmt")
		return callExpr(selectorExpr(goIdent("fmt"), "Sprint"), l.lowerExpr(computed))
	}
	return stringLit(name)
}

func (l *Lowerer) privateKeyExpr(name string) ast.Expr {
	if key, ok := l.privateKeys[name]; ok {
		return goIdent(key)
	}
	return stringLit("#" + name)
}

func (l *Lowerer) lowerEnumDecl(d *hir.EnumDecl) {
	l.jsvalueImport()
	name := l.emitName(d.Symbol)

	// Enum → JSValue object with member properties
	// var EnumName = jsvalue.NewObject()
	// var _ = EnumName.Set("MemberName", jsvalue.NewString("value"))
	l.decls = append(l.decls, varDecl(name, nil,
		callExpr(selectorExpr(goIdent("jsvalue"), "NewObject"))))

	for i, m := range d.Members {
		var val ast.Expr
		if m.Value != nil {
			val = l.lowerExpr(m.Value)
		} else {
			// Numeric enum: use the index
			val = callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
				callExpr(goIdent("float64"), intLit(itoa(i))))
		}
		setCall := callExpr(selectorExpr(goIdent(name), "Set"),
			stringLit(m.Name), val)
		l.initStmts = append(l.initStmts, exprStmt(setCall))
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
	if d.IsDefault {
		l.lowerExportDefault(d)
		return
	}
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

// lowerExportDefault handles `export default ...` declarations.
func (l *Lowerer) lowerExportDefault(d *hir.ExportDecl) {
	if d.Decl == nil {
		return
	}
	switch inner := d.Decl.(type) {
	case *hir.FuncDecl:
		if inner.Symbol != nil {
			inner.Symbol.Exported = true
			l.lowerFuncDecl(inner)
			// Create Default alias pointing to the capitalized name
			goName := l.emitName(inner.Symbol)
			l.decls = append(l.decls, varDecl("Default", nil, goIdent(goName)))
		}
	case *hir.ClassDecl:
		if inner.Symbol != nil {
			inner.Symbol.Exported = true
		}
		l.lowerClassDecl(inner)
	case *hir.VarDecl:
		// export default expr → var Default = expr
		for _, decl := range inner.Declarators {
			var value ast.Expr
			if decl.Init != nil {
				value = l.lowerExpr(decl.Init)
				value = jsvalueWrapLit(value)
			}
			l.decls = append(l.decls, varDecl("Default", nil, value))
			return
		}
	default:
		l.lowerDecl(d.Decl)
	}
}

// synthesizeDefaultExport creates var Default = PrimaryExport when a module
// has named exports but no default export. This matches SWC's interop behavior:
// `import X from 'module'` resolves to the primary named export.
func (l *Lowerer) lowerImportDecl(d *hir.ImportDecl) {
	// Resolve the module path to Go import path + package name
	goImportPath, goPkgName, isKnown := l.resolveModule(d.ModulePath)

	// Look up symbol overrides for known modules
	var overrides map[string]context.SymbolOverride
	if l.ctx != nil {
		if mod := l.ctx.LookupModule(d.ModulePath); mod != nil {
			overrides = mod.SymbolOverrides
		}
	}

	// Process default import
	if d.Default != nil && d.Default.Symbol != nil {
		if isKnown && isGunRuntimePkg(goImportPath) {
			// Default import from Gun runtime module → pkg.AsJSValue
			l.importedSyms[d.Default.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "AsJSValue",
				isTranspiled: false,
			}
		} else if isKnown {
			// Default import from Go stdlib module → namespace (bare pkg ident)
			l.importedSyms[d.Default.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "",
				isTranspiled: false,
			}
		} else {
			goSym := "Default"
			if l.samePackage && isRelativeImport(d.ModulePath) {
				goSym = fileSpecificDefaultName(d.ModulePath)
			}
			if overrides != nil {
				if ov, ok := overrides["default"]; ok {
					goSym = ov.GoSymbol
				}
			}
			l.importedSyms[d.Default.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     goSym,
				isTranspiled: true,
			}
		}
	}

	// Process named imports
	for _, n := range d.Named {
		if n.Symbol == nil {
			continue
		}
		goSym := symbol.Capitalize(n.OriginalName)
		// Same-package imports: check if the symbol is exported from the other file
		// (and thus capitalized), or just an internal reference (stays lowercase).
		if goImportPath == "" {
			capName := symbol.Capitalize(n.OriginalName)
			if l.crossFileExports[capName] {
				goSym = capName // exported from other file → use capitalized
			} else {
				goSym = n.OriginalName // not exported → use original name
			}
		}
		if overrides != nil {
			if ov, ok := overrides[n.OriginalName]; ok {
				goSym = ov.GoSymbol
			}
		}
		l.importedSyms[n.Symbol] = importResolution{
			goImportPath: goImportPath,
			goPkgName:    goPkgName,
			goSymbol:     goSym,
			isTranspiled: !isKnown,
		}
	}

	// Process namespace import
	if d.Namespace != nil && d.Namespace.Symbol != nil {
		if isKnown {
			// import * as fs from "fs" → fs.AsJSValue
			l.importedSyms[d.Namespace.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "AsJSValue",
				isTranspiled: false,
			}
		} else {
			l.importedSyms[d.Namespace.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "", // empty = namespace
				isTranspiled: true,
			}
		}
	}
}

// resolveModule resolves a TS module path to Go import path and package name.
func (l *Lowerer) resolveModule(modulePath string) (goImportPath, goPkgName string, isKnown bool) {
	// Check context-registered known modules
	if l.ctx != nil {
		if mod := l.ctx.LookupModule(modulePath); mod != nil {
			return mod.GoImportPath, mod.GoPkgName, true
		}
	}

	// Transpiled module — compute Go path
	if isRelativeImport(modulePath) {
		if l.samePackage {
			return "", "", false
		}
		clean := strings.TrimSuffix(modulePath, ".ts")
		clean = strings.TrimSuffix(clean, ".js")
		clean = strings.TrimSuffix(clean, ".mjs")
		pkgName := filepath.Base(clean)
		modName := l.moduleName
		if modName == "" {
			modName = "main"
		}
		goPath := path.Clean(modName + "/" + strings.TrimPrefix(clean, "./"))
		return goPath, pkgName, false
	}

	// Third-party package
	pkgName := sanitizeGoPkgName(modulePath)
	if l.moduleName != "" {
		return l.moduleName + "/" + pkgName, pkgName, false
	}
	return pkgName, pkgName, false
}

// collectUsedIdents walks Go AST declarations and returns all identifier names used.
func collectUsedIdents(decls []ast.Decl) map[string]bool {
	used := make(map[string]bool)
	for _, d := range decls {
		ast.Inspect(d, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				used[id.Name] = true
			}
			return true
		})
	}
	return used
}

func isGunRuntimePkg(importPath string) bool {
	return strings.HasPrefix(importPath, "github.com/nnstd/gun/runtime/")
}

func isRelativeImport(p string) bool {
	return strings.HasPrefix(p, ".")
}

func sanitizeGoPkgName(npmName string) string {
	name := strings.TrimPrefix(npmName, "@")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func fileSpecificDefaultName(modulePath string) string {
	base := path.Base(modulePath)
	base = strings.TrimSuffix(base, path.Ext(base))
	name := ""
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			name += string(r)
		} else {
			name += "_"
		}
	}
	if name == "" {
		name = "FileDefault"
	}
	return strings.ToUpper(name[:1]) + name[1:] + "Default"
}

func (l *Lowerer) lowerTopLevelStmt(d *hir.TopLevelStmt) {
	gs := l.lowerStmt(d.Stmt)
	if gs == nil {
		return
	}
	if l.pkgName == "main" {
		mainFn := l.getOrCreateMain()
		mainFn.Body.List = append(mainFn.Body.List, gs)
	} else {
		// Non-main packages: top-level statements go into init()
		l.initStmts = append(l.initStmts, gs)
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

// prescan collects metadata from HIR declarations before lowering.
// Records function param counts and marks exported names.
func (l *Lowerer) prescan(mod *hir.Module) {
	// Mark symbols as exported if they appear in cross-file exports.
	// This ensures that e.g. getCategory in lookup.js gets capitalized to GetCategory
	// when index.js re-exports it.
	if len(l.crossFileExports) > 0 {
		l.markCrossFileExported(mod)
	}

	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *hir.FuncDecl:
			if d.Symbol != nil && d.Symbol.FuncInfo != nil {
				// Already captured by HIR builder
			}
		case *hir.ExportDecl:
			if d.Decl != nil {
				if fd, ok := d.Decl.(*hir.FuncDecl); ok && fd.Symbol != nil {
					fd.Symbol.Exported = true
				}
				if vd, ok := d.Decl.(*hir.VarDecl); ok {
					for _, decl := range vd.Declarators {
						if decl.Symbol != nil {
							decl.Symbol.Exported = true
						}
					}
				}
				if cd, ok := d.Decl.(*hir.ClassDecl); ok && cd.Symbol != nil {
					cd.Symbol.Exported = true
				}
			}
		}
	}
}

// markCrossFileExported marks symbols whose capitalized name matches a cross-file export,
// but ONLY if the original name itself would capitalize to that export name AND no other
// symbol in the same file already has that capitalized name.
func (l *Lowerer) markCrossFileExported(mod *hir.Module) {
	// Build set of all original names in this file to detect conflicts
	localNames := make(map[string]bool)
	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *hir.FuncDecl:
			if d.Symbol != nil {
				localNames[d.Symbol.OriginalName] = true
			}
		case *hir.VarDecl:
			for _, decl := range d.Declarators {
				if decl.Symbol != nil {
					localNames[decl.Symbol.OriginalName] = true
				}
			}
		case *hir.ClassDecl:
			if d.Symbol != nil {
				localNames[d.Symbol.OriginalName] = true
			}
		}
	}

	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *hir.FuncDecl:
			if d.Symbol != nil {
				capName := symbol.Capitalize(d.Symbol.OriginalName)
				// Only mark as exported if the capitalized name is in cross-file exports
				// AND no other local symbol already claims that capitalized name
				if l.crossFileExports[capName] && !localNames[capName] {
					d.Symbol.Exported = true
				}
			}
		case *hir.VarDecl:
			for _, decl := range d.Declarators {
				if decl.Symbol != nil {
					capName := symbol.Capitalize(decl.Symbol.OriginalName)
					if l.crossFileExports[capName] && !localNames[capName] {
						decl.Symbol.Exported = true
					}
				}
			}
		case *hir.ClassDecl:
			if d.Symbol != nil {
				capName := symbol.Capitalize(d.Symbol.OriginalName)
				if l.crossFileExports[capName] && !localNames[capName] {
					d.Symbol.Exported = true
				}
			}
		}
	}
}

// fixInitCycles detects self-referencing package-level variable initializers
// and splits them into forward declarations + init() assignments.
func (l *Lowerer) fixInitCycles(decls []ast.Decl) []ast.Decl {
	var result []ast.Decl
	var initStmts []ast.Stmt

	for _, d := range decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			result = append(result, d)
			continue
		}
		hasCycle := false
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			name := vs.Names[0].Name
			if exprReferencesIdent(vs.Values[0], name) {
				hasCycle = true
				break
			}
		}
		if hasCycle {
			// Split: forward declare + init() assignment
			l.jsvalueImport()
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				typ := vs.Type
				if typ == nil {
					typ = jsValuePtrType()
				}
				fwd := varDecl(vs.Names[0].Name, typ, nil)
				result = append(result, fwd)
				if len(vs.Values) > 0 {
					initStmts = append(initStmts, assignStmt(
						[]ast.Expr{goIdent(vs.Names[0].Name)},
						[]ast.Expr{vs.Values[0]},
					))
				}
			}
		} else {
			result = append(result, d)
		}
	}

	if len(initStmts) > 0 {
		initFn := funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: initStmts})
		result = append(result, initFn)
	}

	return result
}

// exprReferencesIdent checks if an AST expression references an identifier by name.
func exprReferencesIdent(expr ast.Expr, name string) bool {
	found := false
	var walk func(ast.Node, ast.Node)
	walk = func(n ast.Node, parent ast.Node) {
		if found {
			return
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			// Ignore selector field names like pkg.Default; those are property
			// accesses, not references to the variable being initialized.
			if sel, ok := parent.(*ast.SelectorExpr); ok && sel.Sel == id {
				return
			}
			found = true
			return
		}
		ast.Inspect(n, func(child ast.Node) bool {
			if child == nil || child == n {
				return true
			}
			walk(child, n)
			return false
		})
	}
	walk(expr, nil)
	return found
}

// --------------------------------------------------------------------
// Function helpers
// --------------------------------------------------------------------

func (l *Lowerer) lowerFuncBody(params []*hir.Param, body *hir.BlockStmt) *ast.BlockStmt {
	l.insideFunc++
	defer func() { l.insideFunc-- }()
	if body == nil {
		return blockStmt()
	}

	var stmts []ast.Stmt

	// Unpack _args into named parameters
	for i, p := range params {
		// Destructuring parameter: unpack _args[i] then destructure
		if p.Pattern != nil {
			l.jsvalueImport()
			tmpName := fmt.Sprintf("_param%d", i)
			stmts = append(stmts, &ast.DeclStmt{
				Decl: varDecl(tmpName, jsValuePtrType(), nil),
			})
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X: callExpr(goIdent("len"), goIdent("_args")), Op: token.GTR, Y: intLit(itoa(i)),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(tmpName)},
					[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(i))}},
				)),
			})
			// Apply default value for the whole param (e.g. = {})
			if p.Default != nil {
				defVal := l.lowerExpr(p.Default)
				defVal = jsvalueWrapLit(defVal)
				stmts = append(stmts, &ast.IfStmt{
					Cond: &ast.BinaryExpr{X: goIdent(tmpName), Op: token.EQL, Y: goIdent("nil")},
					Body: blockStmt(assignStmt(
						[]ast.Expr{goIdent(tmpName)},
						[]ast.Expr{defVal},
					)),
				})
			}
			// Destructure the parameter
			stmts = append(stmts, l.lowerDestructuring(p.Pattern, goIdent(tmpName), true)...)
			continue
		}
		if p.Symbol == nil {
			continue
		}
		name := l.emitName(p.Symbol)
		if p.Rest {
			// Rest param: name := jsvalue.NewArray(_args[i:]...)
			l.jsvalueImport()
			restCall := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"),
				&ast.SliceExpr{X: goIdent("_args"), Low: intLit(itoa(i))})
			restCall.Ellipsis = 1 // spread the slice
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{goIdent(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{restCall},
			})
		} else {
			// Bounds-checked unpacking:
			// var name *jsvalue.JSValue
			// if len(_args) > i { name = _args[i] }
			l.jsvalueImport()
			stmts = append(stmts, &ast.DeclStmt{
				Decl: varDecl(name, jsValuePtrType(), nil),
			})
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  callExpr(goIdent("len"), goIdent("_args")),
					Op: token.GTR,
					Y:  intLit(itoa(i)),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(name)},
					[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(i))}},
				)),
			})

			// Default value or nil→undefined handling
			if p.Default != nil {
				defVal := l.lowerExpr(p.Default)
				defVal = jsvalueWrapLit(defVal)
				stmts = append(stmts, &ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X:  goIdent(name),
						Op: token.EQL,
						Y:  goIdent("nil"),
					},
					Body: blockStmt(assignStmt(
						[]ast.Expr{goIdent(name)},
						[]ast.Expr{defVal},
					)),
				})
			}
		}
	}

	// Hoist function declarations to top of body (JS hoisting semantics)
	hoistedBody := hoistFunctions(body.Stmts)

	// Lower body statements, flattening inline blocks (multi-declarator VarDecl)
	for _, s := range hoistedBody {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		if block, ok := gs.(*ast.BlockStmt); ok {
			if true {
				stmts = append(stmts, block.List...)
				continue
			}
		}
		stmts = append(stmts, gs)
	}

	// Ensure trailing return nil if function body doesn't end with a return
	if !endsWithReturn(stmts) {
		stmts = append(stmts, returnStmt(goIdent("nil")))
	}

	// Forward-declare variables used before their := definition
	stmts = forwardDeclareVars(stmts)

	// Replace unused := variables with _ to satisfy Go's "declared and not used" rule
	stmts = eliminateUnusedVars(stmts)

	return &ast.BlockStmt{List: stmts}
}

// endsWithReturn checks if a statement list ends with a return statement.
func endsWithReturn(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	last := stmts[len(stmts)-1]
	switch s := last.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		// if-else where both branches return
		if s.Else != nil && s.Body != nil {
			if !endsWithReturn(s.Body.List) {
				return false
			}
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				return endsWithReturn(e.List)
			}
		}
	}
	return false
}

// lowerMethodBody is like lowerFuncBody but prepends `this := _args[0]`
// and unpacks remaining params from _args[1:] offset.
func (l *Lowerer) lowerMethodBody(params []*hir.Param, body *hir.BlockStmt) *ast.BlockStmt {
	l.insideFunc++
	l.insideMethod++
	defer func() { l.insideFunc--; l.insideMethod-- }()
	if body == nil {
		return blockStmt()
	}
	var stmts []ast.Stmt
	l.jsvalueImport()

	// Only declare `this` if the method body references it
	if hirBodyUsesThis(body) {
		stmts = append(stmts, &ast.DeclStmt{
			Decl: varDecl("this", jsValuePtrType(), nil),
		})
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: callExpr(goIdent("len"), goIdent("_args")), Op: token.GTR, Y: intLit("0"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent("this")},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}},
			)),
		})
	}

	// Unpack named params from _args[1+i]
	for i, p := range params {
		idx := i + 1 // offset by 1 for 'this'
		// Destructuring parameter: unpack _args[idx] then destructure
		if p.Pattern != nil {
			tmpName := fmt.Sprintf("_param%d", idx)
			stmts = append(stmts, &ast.DeclStmt{
				Decl: varDecl(tmpName, jsValuePtrType(), nil),
			})
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X: callExpr(goIdent("len"), goIdent("_args")), Op: token.GTR, Y: intLit(itoa(idx)),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(tmpName)},
					[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(idx))}},
				)),
			})
			stmts = append(stmts, l.lowerDestructuring(p.Pattern, goIdent(tmpName), true)...)
			continue
		}
		if p.Symbol == nil {
			continue
		}
		name := l.emitName(p.Symbol)
		if p.Rest {
			restCall := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"),
				&ast.SliceExpr{X: goIdent("_args"), Low: intLit(itoa(idx))})
			restCall.Ellipsis = 1
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{goIdent(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{restCall},
			})
			continue
		}
		stmts = append(stmts, &ast.DeclStmt{
			Decl: varDecl(name, jsValuePtrType(), nil),
		})
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: callExpr(goIdent("len"), goIdent("_args")), Op: token.GTR, Y: intLit(itoa(idx)),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(name)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(idx))}},
			)),
		})

		// Default value
		if p.Default != nil {
			defVal := l.lowerExpr(p.Default)
			defVal = jsvalueWrapLit(defVal)
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{X: goIdent(name), Op: token.EQL, Y: goIdent("nil")},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(name)},
					[]ast.Expr{defVal},
				)),
			})
		}
	}

	// Lower body statements, flattening inline blocks
	for _, s := range body.Stmts {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		if block, ok := gs.(*ast.BlockStmt); ok {
			if true {
				stmts = append(stmts, block.List...)
				continue
			}
		}
		stmts = append(stmts, gs)
	}

	// Ensure trailing return nil
	if !endsWithReturn(stmts) {
		stmts = append(stmts, returnStmt(goIdent("nil")))
	}

	// Forward-declare variables used before their := definition
	stmts = forwardDeclareVars(stmts)

	// Replace unused := variables with _ to satisfy Go's "declared and not used" rule
	stmts = eliminateUnusedVars(stmts)

	return &ast.BlockStmt{List: stmts}
}

// forwardDeclareVars scans Go statements for variables used before their :=
// declaration. Adds `var name *jsvalue.JSValue` at top and changes := to =.
// Also hoists function-valued assignments to the top (JS function hoisting).
func forwardDeclareVars(stmts []ast.Stmt) []ast.Stmt {
	// Step 1: Identify all := declarations
	declared := make(map[string]int) // name → index of first := decl
	bareVarDecls := make(map[string]int)
	for i, s := range stmts {
		if assign, ok := s.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			for _, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					if _, exists := declared[id.Name]; !exists {
						declared[id.Name] = i
					}
				}
			}
		}
		if decl, ok := s.(*ast.DeclStmt); ok {
			if gd, ok := decl.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if _, exists := bareVarDecls[name.Name]; !exists {
								bareVarDecls[name.Name] = i
							}
						}
					}
				}
			}
		}
	}
	if len(declared) == 0 && len(bareVarDecls) == 0 {
		return stmts
	}

	// Step 2: Detect forward references (variables used before their := position)
	forwarded := make(map[string]bool)
	for name, declIdx := range declared {
		for i := 0; i < declIdx; i++ {
			if stmtReferencesIdent(stmts[i], name) {
				forwarded[name] = true
				break
			}
		}
	}
	for name, declIdx := range bareVarDecls {
		for i := 0; i < declIdx; i++ {
			if stmtReferencesIdent(stmts[i], name) {
				forwarded[name] = true
				break
			}
		}
	}
	if len(forwarded) == 0 {
		return stmts
	}

	// Step 5: Create var declarations at top and change := to =
	var fwd []ast.Stmt
	for name := range forwarded {
		fwd = append(fwd, &ast.DeclStmt{Decl: varDecl(name, jsValuePtrType(), nil)})
	}
	for i, s := range stmts {
		if assign, ok := s.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			for _, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && forwarded[id.Name] {
					assign.Tok = token.ASSIGN
					break
				}
			}
		}
		stmts[i] = s
	}

	// Remove original bare var declarations that have now been hoisted.
	var filtered []ast.Stmt
	for _, s := range stmts {
		if decl, ok := s.(*ast.DeclStmt); ok {
			if gd, ok := decl.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				allForwarded := true
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if !forwarded[name.Name] {
								allForwarded = false
							}
						}
					}
				}
				if allForwarded {
					continue
				}
			}
		}
		filtered = append(filtered, s)
	}
	stmts = filtered

	// Final step: hoist forward-declared function assignments right after var
	// declarations (JS function hoisting). Only hoist functions that were
	// actually forward-declared — these are the ones whose := was split.
	var hoisted []ast.Stmt
	var remaining []ast.Stmt
	for _, s := range stmts {
		if decl, ok := s.(*ast.DeclStmt); ok {
			if gd, ok := decl.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				keep := false
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if !forwarded[name.Name] {
								keep = true
							}
						}
					}
				}
				if !keep {
					continue
				}
			}
		}
		if assign, ok := s.(*ast.AssignStmt); ok && assign.Tok == token.ASSIGN {
			if len(assign.Lhs) == 1 {
				if id, ok := assign.Lhs[0].(*ast.Ident); ok && forwarded[id.Name] && isFuncValuedAssign(assign) {
					hoisted = append(hoisted, s)
					continue
				}
			}
		}
		remaining = append(remaining, s)
	}
	if len(hoisted) > 0 {
		// Hoisted function bodies may reference variables (argv, flags, etc.)
		// that are assigned later. These variables need var declarations before
		// the hoisted code. Scan hoisted functions for referenced names and
		// add var decls for any that aren't already forward-declared.
		existingDecls := make(map[string]bool)
		for name := range forwarded {
			existingDecls[name] = true
		}
		// Find all = assignments in remaining (these are variable assignments)
		assignedNames := make(map[string]bool)
		for _, s := range remaining {
			if assign, ok := s.(*ast.AssignStmt); ok {
				for _, lhs := range assign.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						assignedNames[id.Name] = true
					}
				}
			}
		}
		// Check which names hoisted functions reference that aren't declared yet
		var extraDecls []ast.Stmt
		for _, s := range hoisted {
			ast.Inspect(s, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && !existingDecls[id.Name] && assignedNames[id.Name] {
					existingDecls[id.Name] = true
					extraDecls = append(extraDecls, &ast.DeclStmt{Decl: varDecl(id.Name, jsValuePtrType(), nil)})
					// Change the corresponding := in remaining to = since we forward-declared it
					for _, rs := range remaining {
						if assign, ok := rs.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
							for _, lhs := range assign.Lhs {
								if lid, ok := lhs.(*ast.Ident); ok && lid.Name == id.Name {
									assign.Tok = token.ASSIGN
								}
							}
						}
					}
				}
				return true
			})
		}
		fwd = append(fwd, extraDecls...)
		return append(append(fwd, hoisted...), remaining...)
	}

	return append(fwd, stmts...)
}

// eliminateUnusedVars performs SWC-style write/read analysis on an entire
// function body. Variables that are written (declared or assigned) but never
// read are eliminated: var decls are removed, := becomes _ =, = becomes _ =.
func eliminateUnusedVars(stmts []ast.Stmt) []ast.Stmt {
	// Only locals declared in this function body are eligible for elimination.
	// Assignments to outer/package scope vars may have effects outside the current
	// body and must not be rewritten to `_ =`.
	locals := make(map[string]bool)
	collectLocalDecls(stmts, locals)

	// Phase 1: Collect writes and reads across the entire body (recursively)
	writes := make(map[string]bool)
	reads := make(map[string]bool)
	for _, s := range stmts {
		collectWritesAndReads(s, writes, reads, locals)
	}

	// Phase 2: Find write-only variables (written but never read)
	unused := make(map[string]bool)
	for name := range writes {
		if !reads[name] {
			unused[name] = true
		}
	}
	if len(unused) == 0 {
		return stmts
	}

	// Phase 3: Eliminate unused variables throughout the AST
	eliminateUnusedInStmts(stmts, unused)

	// Phase 4: Remove bare `var x *T` DeclStmts for unused vars
	return filterUnusedDecls(stmts, unused)
}

func collectLocalDecls(stmts []ast.Stmt, locals map[string]bool) {
	for _, s := range stmts {
		ast.Inspect(s, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if n.Tok != token.DEFINE {
					return true
				}
				for _, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
						locals[id.Name] = true
					}
				}
			case *ast.GenDecl:
				if n.Tok != token.VAR {
					return true
				}
				for _, spec := range n.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if name.Name != "_" {
								locals[name.Name] = true
							}
						}
					}
				}
			}
			return true
		})
	}
}

// collectWritesAndReads walks a statement recursively, recording which
// identifiers appear in write positions (LHS of assignment, var decl names)
// vs read positions (everything else).
func collectWritesAndReads(node ast.Node, writes, reads, locals map[string]bool) {
	// Track idents that are in write positions so we skip them as reads
	writeIdents := make(map[*ast.Ident]bool)

	// First pass: mark all write-position idents
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && (n.Tok == token.DEFINE || locals[id.Name]) {
					writeIdents[id] = true
					if id.Name != "_" {
						writes[id.Name] = true
					}
				}
			}
		case *ast.GenDecl:
			if n.Tok == token.VAR {
				for _, spec := range n.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							writeIdents[name] = true
							if name.Name != "_" {
								writes[name.Name] = true
							}
						}
					}
				}
			}
		}
		return true
	})

	// Second pass: all idents NOT in write positions are reads
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && !writeIdents[id] && id.Name != "_" {
			reads[id.Name] = true
		}
		return true
	})
}

// collectReads walks an expression and records all identifiers as reads.
func collectReads(node ast.Node, reads map[string]bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name != "_" {
			reads[id.Name] = true
		}
		return true
	})
}

// eliminateUnusedInStmts walks statements recursively and blanks out
// unused variable references in assignment LHS positions.
func eliminateUnusedInStmts(stmts []ast.Stmt, unused map[string]bool) {
	for _, s := range stmts {
		ast.Inspect(s, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				allBlank := true
				for i, lhs := range assign.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && unused[id.Name] {
						assign.Lhs[i] = goIdent("_")
					}
					if id, ok := assign.Lhs[i].(*ast.Ident); !ok || id.Name != "_" {
						allBlank = false
					}
				}
				if allBlank && assign.Tok == token.DEFINE {
					assign.Tok = token.ASSIGN
				}
			}
			return true
		})
	}
}

// filterUnusedDecls removes `var x *T` DeclStmts where x is unused.
// Works recursively on nested BlockStmts.
func filterUnusedDecls(stmts []ast.Stmt, unused map[string]bool) []ast.Stmt {
	var result []ast.Stmt
	for _, s := range stmts {
		// Recurse into block statements
		ast.Inspect(s, func(n ast.Node) bool {
			if block, ok := n.(*ast.BlockStmt); ok && block != nil {
				block.List = filterUnusedDecls(block.List, unused)
			}
			return true
		})
		// Check if this is a removable var decl
		if decl, ok := s.(*ast.DeclStmt); ok {
			if gd, ok := decl.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				allUnused := true
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if !unused[name.Name] {
								allUnused = false
							}
						}
					}
				}
				if allUnused {
					continue // skip this decl entirely
				}
			}
		}
		result = append(result, s)
	}
	return result
}

// isFuncValuedAssign checks if an assignment's RHS is a jsvalue.NewFunction() call
// (possibly with .MarkAsMethod() chained).
func isFuncValuedAssign(assign *ast.AssignStmt) bool {
	if len(assign.Rhs) != 1 {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	fn := call.Fun
	if sel, ok := fn.(*ast.SelectorExpr); ok && sel.Sel.Name == "MarkAsMethod" {
		if inner, ok := sel.X.(*ast.CallExpr); ok {
			fn = inner.Fun
		}
	}
	if sel, ok := fn.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			return id.Name == "jsvalue" && sel.Sel.Name == "NewFunction"
		}
	}
	return false
}

func stmtReferencesIdent(s ast.Stmt, name string) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		if found {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// hirBodyUsesThis checks if an HIR block references `this`.
func hirBodyUsesThis(body *hir.BlockStmt) bool {
	if body == nil {
		return false
	}
	found := false
	var walk func(e hir.Expr)
	var walkStmt func(s hir.Stmt)
	walk = func(e hir.Expr) {
		if found || e == nil {
			return
		}
		switch e := e.(type) {
		case *hir.ThisExpr:
			found = true
		case *hir.BinaryExpr:
			walk(e.Left)
			walk(e.Right)
		case *hir.UnaryExpr:
			walk(e.Operand)
		case *hir.CallExpr:
			walk(e.Func)
			for _, a := range e.Args {
				walk(a)
			}
		case *hir.MemberExpr:
			walk(e.Object)
		case *hir.ComputedMemberExpr:
			walk(e.Object)
			walk(e.Property)
		case *hir.AssignExpr:
			walk(e.Left)
			walk(e.Right)
		case *hir.TernaryExpr:
			walk(e.Cond)
			walk(e.Then)
			walk(e.Else)
		case *hir.ArrayLiteral:
			for _, el := range e.Elements {
				walk(el)
			}
		case *hir.ObjectLiteral:
			for _, p := range e.Properties {
				walk(p.Value)
			}
		case *hir.SpreadExpr:
			walk(e.Value)
		case *hir.NewExpr:
			walk(e.Callee)
			for _, a := range e.Args {
				walk(a)
			}
		case *hir.UpdateExpr:
			walk(e.Operand)
		case *hir.ArrowFunc:
			if e.Body != nil {
				for _, st := range e.Body.Stmts {
					walkStmt(st)
				}
			}
			if e.ExprBody != nil {
				walk(e.ExprBody)
			}
		case *hir.FuncExpr:
			if e.Body != nil {
				for _, st := range e.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.TemplateLiteral:
			for _, p := range e.Parts {
				walk(p)
			}
		case *hir.SequenceExpr:
			for _, ex := range e.Exprs {
				walk(ex)
			}
		case *hir.ParenExpr:
			walk(e.Expr)
		}
	}
	walkStmt = func(s hir.Stmt) {
		if found || s == nil {
			return
		}
		switch s := s.(type) {
		case *hir.ExprStmt:
			walk(s.Expr)
		case *hir.ReturnStmt:
			walk(s.Value)
		case *hir.VarDecl:
			for _, d := range s.Declarators {
				walk(d.Init)
			}
		case *hir.IfStmt:
			walk(s.Cond)
			if s.Then != nil {
				for _, st := range s.Then.Stmts {
					walkStmt(st)
				}
			}
			if s.Else != nil {
				walkStmt(s.Else)
			}
		case *hir.ForStmt:
			walkStmt(s.Init)
			walk(s.Cond)
			walk(s.Post)
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.WhileStmt:
			walk(s.Cond)
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.BlockStmt:
			for _, st := range s.Stmts {
				walkStmt(st)
			}
		case *hir.ThrowStmt:
			walk(s.Value)
		case *hir.SwitchStmt:
			walk(s.Tag)
			for _, c := range s.Cases {
				for _, st := range c.Body {
					walkStmt(st)
				}
			}
		case *hir.TryCatchStmt:
			if s.Try != nil {
				for _, st := range s.Try.Stmts {
					walkStmt(st)
				}
			}
			if s.Catch != nil && s.Catch.Body != nil {
				for _, st := range s.Catch.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.DoWhileStmt:
			walk(s.Cond)
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.ForOfStmt:
			walk(s.Value)
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					walkStmt(st)
				}
			}
		case *hir.ForInStmt:
			walk(s.Value)
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					walkStmt(st)
				}
			}
		}
	}
	for _, s := range body.Stmts {
		walkStmt(s)
		if found {
			return true
		}
	}
	return false
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
