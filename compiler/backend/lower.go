package backend

import (
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
	imports          map[string]string                    // Go import path → alias
	decls            []ast.Decl
	importedSyms     map[*symbol.Symbol]importResolution  // how each imported symbol resolves to Go
	moduleName       string                               // Go module name for relative import resolution
	samePackage      bool                                 // treat relative imports as same-package refs
	varTypes         map[string]string                    // variable name → module type (e.g. "hono")
	crossFileExports map[string]bool                      // Go names from other files (prevents .Get() dispatch)
	initStmts        []ast.Stmt                           // statements for init() function
	pkgName          string                               // Go package name
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
		varTypes:     make(map[string]string),
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

	// Fix init cycles: split self-referencing vars into forward decl + init()
	l.decls = l.fixInitCycles(l.decls)

	// Ensure main() exists for runnable packages
	if mod.Package == "main" {
		l.getOrCreateMain()
	}

	// Emit init() for collected setup statements (class methods, enum members, etc.)
	if len(l.initStmts) > 0 {
		l.decls = append(l.decls, funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: l.initStmts}))
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

	// Build constructor function body
	// Constructor signature: func(this *jsvalue.JSValue, _args ...*jsvalue.JSValue) *jsvalue.JSValue
	// Parameters are unpacked from _args (offset 0, since 'this' is separate in NewClass)
	var ctorBody *ast.BlockStmt
	if d.Constructor != nil {
		ctorBody = l.lowerFuncBody(d.Constructor.Params, d.Constructor.Body)
	} else {
		ctorBody = blockStmt()
	}
	// Ensure constructor returns nil (constructors return this implicitly)
	ctorBody.List = append(ctorBody.List, returnStmt(goIdent("nil")))
	// Constructor has signature: func(this *jsvalue.JSValue, _args ...*jsvalue.JSValue) *jsvalue.JSValue
	ctorLit := &ast.FuncLit{
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

	// jsvalue.NewClass(ctorFn, parent) — parent is nil if no extends
	var parentExpr ast.Expr = goIdent("nil")
	if d.Parent != nil {
		parentExpr = l.lowerExpr(d.Parent)
	}
	classArgs := []ast.Expr{ctorLit, parentExpr}
	newClassCall := callExpr(selectorExpr(goIdent("jsvalue"), "NewClass"), classArgs...)
	l.decls = append(l.decls, varDecl(name, nil, newClassCall))

	// Static properties
	for _, prop := range d.Properties {
		if !prop.IsStatic || prop.Value == nil {
			continue
		}
		val := l.lowerExpr(prop.Value)
		val = jsvalueWrapLit(val)
		setCall := callExpr(selectorExpr(goIdent(name), "Set"), stringLit(prop.Name), val)
		l.initStmts = append(l.initStmts, exprStmt(setCall))
	}

	// Methods: ClassName.Get("prototype").Set("method", jsvalue.NewFunction(...))
	for _, m := range d.Methods {
		// Instance methods: first _args element is 'this'
		var methodBody *ast.BlockStmt
		if !m.IsStatic {
			// Prepend: this := _args[0]; remaining params from _args[1:]
			methodBody = l.lowerMethodBody(m.Params, m.Body)
		} else {
			methodBody = l.lowerFuncBody(m.Params, m.Body)
		}
		methodLit := l.wrapAsJSValueFunc(m.Params, methodBody)
		methodFn := callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), methodLit)

		var setCall ast.Expr
		if m.IsStatic {
			setCall = callExpr(selectorExpr(goIdent(name), "Set"), stringLit(m.Name), methodFn)
		} else {
			proto := callExpr(selectorExpr(goIdent(name), "Get"), stringLit("prototype"))
			setCall = callExpr(selectorExpr(proto, "Set"), stringLit(m.Name), methodFn)
		}
		l.initStmts = append(l.initStmts, exprStmt(setCall))
	}
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
		if isKnown {
			// Default import from known module acts as namespace: import fs from "fs" → package alias
			l.importedSyms[d.Default.Symbol] = importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "", // empty = namespace
				isTranspiled: false,
			}
		} else {
			goSym := "Default"
			if l.samePackage && isRelativeImport(d.ModulePath) && !strings.HasPrefix(d.ModulePath, "..") {
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
		// Same-package imports: don't capitalize (both files in same Go package)
		if goImportPath == "" {
			goSym = n.OriginalName
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
		l.importedSyms[d.Namespace.Symbol] = importResolution{
			goImportPath: goImportPath,
			goPkgName:    goPkgName,
			goSymbol:     "", // empty = namespace
			isTranspiled: !isKnown,
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
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
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

	// Lower body statements, flattening inline blocks (multi-declarator VarDecl)
	for _, s := range body.Stmts {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		if block, ok := gs.(*ast.BlockStmt); ok {
			if _, isHIRBlock := s.(*hir.BlockStmt); !isHIRBlock {
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
	if body == nil {
		return blockStmt()
	}
	var stmts []ast.Stmt
	l.jsvalueImport()

	// this := _args[0]
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

	// Unpack named params from _args[1+i]
	for i, p := range params {
		if p.Symbol == nil {
			continue
		}
		name := l.emitName(p.Symbol)
		idx := i + 1 // offset by 1 for 'this'
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
	}

	// Lower body statements, flattening inline blocks
	for _, s := range body.Stmts {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		if block, ok := gs.(*ast.BlockStmt); ok {
			if _, isHIRBlock := s.(*hir.BlockStmt); !isHIRBlock {
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
