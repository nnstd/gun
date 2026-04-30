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
	useAsJSValue bool
	moduleValue  string
	jsExportName string
	modulePath   string
}

// CrossFileExport describes a symbol exported from another file in the same package.
type CrossFileExport struct {
	OriginalName string
	GoName       string
	IsJSValue    bool
}

// Lowerer converts an HIR Module into a go/ast.File.
type Lowerer struct {
	symtab              *symbol.Table
	ctx                 *context.TranspilerContext
	optLevel            context.OptLevel
	imports             map[string]string // Go import path → alias
	decls               []ast.Decl
	importedSyms        map[*symbol.Symbol]importResolution // how each imported symbol resolves to Go
	importedNames       map[string]importResolution         // fallback for unresolved imported identifiers
	moduleName          string                              // Go module name for relative import resolution
	samePackage         bool                                // treat relative imports as same-package refs
	varTypes            map[string]string                   // variable name → module type (e.g. "hono")
	reservedNames       map[string]bool
	importNameMap       map[string]string
	exportAliasMap      map[string]string
	localAliasMap       map[symbol.ID]string
	namespaceAlias      string
	namespaceEntries    map[string]string
	topLevelNames       map[string]string
	eagerVarInits       map[symbol.ID]bool
	emittedExportNames  map[string]bool
	crossFileExports    map[string]bool // Go names from other files (prevents .Get() dispatch)
	initStmts           []ast.Stmt      // statements for init() function
	pkgName             string          // Go package name
	currentClassName    string          // set during class constructor/method lowering
	insideFunc          int             // >0 when inside a function body
	insideMethod        int             // >0 when inside a method body (_args[0] is this)
	privateKeys         map[string]string
	currentClassBrand   string
	syntheticCounter    int
	needsBunWait        bool
	sourcePath          string
	asyncTempSymbols    []*symbol.Symbol
	hasTopLevelAwait    bool
	topLevelAwaitStmts  []hir.Stmt
	arenaEnabled        bool
	disableArenaCount   int
	hasArenaVar         int
	cpuProfile          *CPUProfileConfig
	profileRuntimeAlias string
}

// Lower converts an HIR module to a Go AST file.
func Lower(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool, optLevel context.OptLevel) *ast.File {
	return LowerWithCPUProfile(mod, ctx, moduleName, samePackageImports, nil, optLevel)
}

// LowerWithCPUProfile converts an HIR module to a Go AST file with optional
// generated-main CPU profiling support.
func LowerWithCPUProfile(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool, cpuProfile *CPUProfileConfig, optLevel context.OptLevel) *ast.File {
	return LowerWithExportsAndCPUProfile(mod, ctx, moduleName, samePackageImports, nil, nil, nil, nil, nil, "", nil, cpuProfile, optLevel)
}

// LowerWithExports converts an HIR module to a Go AST file with knowledge of
// symbols exported from other files in the same package.
func LowerWithExports(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool, crossFileExports []CrossFileExport, reservedNames []string, importNameMap map[string]string, exportAliasMap map[string]string, localAliasMap map[symbol.ID]string, namespaceAlias string, namespaceEntries map[string]string, optLevel context.OptLevel) *ast.File {
	return LowerWithExportsAndCPUProfile(mod, ctx, moduleName, samePackageImports, crossFileExports, reservedNames, importNameMap, exportAliasMap, localAliasMap, namespaceAlias, namespaceEntries, nil, optLevel)
}

// LowerWithExportsAndCPUProfile converts an HIR module to a Go AST file with
// knowledge of same-package exports plus optional generated-main CPU profiling support.
func LowerWithExportsAndCPUProfile(mod *hir.Module, ctx *context.TranspilerContext, moduleName string, samePackageImports bool, crossFileExports []CrossFileExport, reservedNames []string, importNameMap map[string]string, exportAliasMap map[string]string, localAliasMap map[symbol.ID]string, namespaceAlias string, namespaceEntries map[string]string, cpuProfile *CPUProfileConfig, optLevel context.OptLevel) *ast.File {
	cfe := make(map[string]bool)
	for _, exp := range crossFileExports {
		cfe[exp.GoName] = true
	}
	reserved := make(map[string]bool)
	for _, name := range reservedNames {
		reserved[name] = true
	}
	l := &Lowerer{
		symtab:             mod.SymbolTable,
		ctx:                ctx,
		optLevel:           optLevel,
		imports:            make(map[string]string),
		importedSyms:       make(map[*symbol.Symbol]importResolution),
		importedNames:      make(map[string]importResolution),
		moduleName:         moduleName,
		samePackage:        samePackageImports,
		reservedNames:      reserved,
		importNameMap:      importNameMap,
		exportAliasMap:     exportAliasMap,
		localAliasMap:      localAliasMap,
		namespaceAlias:     namespaceAlias,
		namespaceEntries:   namespaceEntries,
		topLevelNames:      make(map[string]string),
		eagerVarInits:      make(map[symbol.ID]bool),
		emittedExportNames: make(map[string]bool),
		crossFileExports:   cfe,
		pkgName:            mod.Package,
		varTypes:           make(map[string]string),
		sourcePath:         mod.SourcePath,
		hasTopLevelAwait:   mod.HasTopLevelAwait,
		arenaEnabled:       optLevel >= context.O2,
		cpuProfile:         cpuProfile,
	}

	// Reserve cross-file export names in the symbol table so local symbols
	// get suffixed on collision instead of redeclaring.
	for name := range l.crossFileExports {
		l.symtab.ReserveNameStr(name)
	}
	for name := range l.reservedNames {
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

	if l.namespaceAlias != "" {
		l.jsvalueImport()
		props := make([]ast.Expr, 0, len(l.namespaceEntries)*2)
		for exportName, alias := range l.namespaceEntries {
			props = append(props, stringLit(exportName), goIdent(alias))
		}
		// Declare namespace var, but initialize in init() so all referenced vars
		// (which may themselves be set in init()) are available.
		l.decls = append(l.decls, varDecl(l.namespaceAlias, jsValuePtrType(), nil))
		l.initStmts = append(l.initStmts, assignStmt(
			[]ast.Expr{goIdent(l.namespaceAlias)},
			[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), props...)},
		))
	}

	// Fix init cycles: split self-referencing vars into forward decl + init()
	l.decls = l.fixInitCycles(l.decls)

	// Handle top-level await: wrap main body in async state machine
	if mod.Package == "main" && l.hasTopLevelAwait && len(l.topLevelAwaitStmts) > 0 {
		asyncBody := l.lowerAsyncFuncBody(nil, &hir.BlockStmt{Stmts: l.topLevelAwaitStmts}, 0, false)
		// Replace final return with expression statement (main returns nothing)
		if len(asyncBody.List) > 0 {
			if ret, ok := asyncBody.List[len(asyncBody.List)-1].(*ast.ReturnStmt); ok && len(ret.Results) > 0 {
				asyncBody.List[len(asyncBody.List)-1] = exprStmt(ret.Results[0])
			}
		}
		mainFn := l.getOrCreateMain()
		// Keep defer error.RecoverMain() if present
		var prefix []ast.Stmt
		if len(mainFn.Body.List) > 0 {
			if _, ok := mainFn.Body.List[0].(*ast.DeferStmt); ok {
				prefix = mainFn.Body.List[:1]
			}
		}
		mainFn.Body.List = append(prefix, asyncBody.List...)
		l.addAliasedImport("github.com/nnstd/gun/runtime/eventloop", "eventloop")
		mainFn.Body.List = append(mainFn.Body.List, exprStmt(callExpr(
			selectorExpr(selectorExpr(goIdent("eventloop"), "Default"), "Run"),
		)))
	} else if mod.Package == "main" {
		// Ensure main() exists for runnable packages
		l.getOrCreateMain()
		l.addAliasedImport("github.com/nnstd/gun/runtime/eventloop", "eventloop")
		if mainFn := l.findMainFunc(); mainFn != nil {
			mainFn.Body.List = append(mainFn.Body.List, exprStmt(callExpr(
				selectorExpr(selectorExpr(goIdent("eventloop"), "Default"), "Run"),
			)))
		}
	}
	l.injectCPUProfileMain()

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
		if pkgIdent != "_" && !usedIdents[pkgIdent] {
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
	if alias, ok := l.localAliasMap[sym.ID]; ok {
		return alias
	}
	return l.symtab.EmitName(sym)
}
func (l *Lowerer) registerTopLevelName(sym *symbol.Symbol) {
	if sym == nil {
		return
	}
	l.topLevelNames[sym.OriginalName] = l.emitName(sym)
}

// --------------------------------------------------------------------
// Declarations
// --------------------------------------------------------------------

func (l *Lowerer) lowerDecl(d hir.Decl) {
	switch d := d.(type) {
	case *hir.FuncDecl:
		l.lowerFuncDecl(d)
	case *hir.VarDecl:
		if l.hasTopLevelAwait && l.pkgName == "main" && hir.VarDeclContainsAwait(d) {
			l.topLevelAwaitStmts = append(l.topLevelAwaitStmts, d)
		} else {
			l.lowerVarDecl(d)
		}
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

	// main and init stay as Go func declarations.
	// Check OriginalName because Sanitize may rename "init" → "init_".
	origName := name
	if d.Symbol != nil {
		origName = d.Symbol.OriginalName
	}
	if origName == "main" || origName == "init" {
		name = origName // Use the original name for Go func declaration
		body := l.lowerBlock(d.Body)
		body = l.instrumentProfiledBody(name, d.Span, body)
		if d.IsAsync {
			asyncName := l.nextSyntheticName("_" + name + "_async")
			l.jsvalueImport()
			asyncBody := l.lowerAsyncFuncBody(d.Params, d.Body, 0, false)
			asyncBody = l.instrumentProfiledBody(asyncName, d.Span, asyncBody)
			asyncLit := setFuncLitPos(l.wrapAsJSValueFunc(d.Params, asyncBody), d.Span)
			asyncVal := callExpr(selectorExpr(l.generatedFunctionValue(asyncName, d.Span, asyncLit), "MarkAsAsync"))
			l.decls = append(l.decls, setDeclPos(varDecl(asyncName, nil, asyncVal), d.Span))
			body = blockStmt(
				assignDefine(
					[]ast.Expr{goIdent("_promise")},
					[]ast.Expr{callExpr(selectorExpr(goIdent(asyncName), "Call"))},
				),
				exprStmt(callExpr(
					selectorExpr(goIdent("_promise"), "MethodCall"),
					stringLit("catch"),
					callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), &ast.FuncLit{
						Type: &ast.FuncType{
							Params: fieldList(&ast.Field{
								Names: []*ast.Ident{goIdent("_args")},
								Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
							}),
							Results: fieldList(goField("", jsValuePtrType())),
						},
						Body: blockStmt(
							&ast.IfStmt{
								Cond: &ast.BinaryExpr{
									X:  callExpr(goIdent("len"), goIdent("_args")),
									Op: token.GTR,
									Y:  intLit("0"),
								},
								Body: blockStmt(exprStmt(callExpr(goIdent("panic"), &ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}))),
							},
							exprStmt(callExpr(goIdent("panic"), callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined")))),
							returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
						),
					}),
				)),
			)
		}
		fd := setFuncDeclPos(funcDecl(name, fieldList(), nil, body), d.Span)
		l.decls = append(l.decls, fd)
		return
	}

	// All other functions become jsvalue.NewFunction vars
	l.jsvalueImport()
	var body *ast.BlockStmt
	methodLike := d.Body != nil && hirBodyUsesThis(d.Body)
	if d.IsAsync {
		body = l.lowerAsyncFuncBody(d.Params, d.Body, 0, false)
	} else {
		body = l.lowerFuncBody(d.Params, d.Body)
		if methodLike {
			body = l.lowerMethodBody(d.Params, d.Body)
		}
	}
	body = l.instrumentProfiledBody(name, d.Span, body)
	fnLit := setFuncLitPos(l.wrapAsJSValueFunc(d.Params, body), d.Span)
	fnVal := l.generatedFunctionValue(name, d.Span, fnLit)
	if d.IsAsync {
		fnVal = callExpr(selectorExpr(fnVal, "MarkAsAsync"))
	}
	if methodLike {
		fnVal = callExpr(selectorExpr(fnVal, "MarkAsMethod"))
	}
	l.decls = append(l.decls, setDeclPos(varDecl(name, nil, fnVal), d.Span))
}
func (l *Lowerer) lowerVarDecl(d *hir.VarDecl) {
	for _, decl := range d.Declarators {
		if decl.Pattern != nil && decl.Init != nil {
			l.jsvalueImport()
			for _, sym := range collectPatternSymbols(decl.Pattern) {
				l.decls = append(l.decls, varDecl(l.emitName(sym), jsValuePtrType(), nil))
			}
			l.initStmts = append(l.initStmts, l.lowerDestructuring(decl.Pattern, l.lowerExpr(decl.Init), false)...)
			continue
		}
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

func collectPatternSymbols(pat hir.Pattern) []*symbol.Symbol {
	var out []*symbol.Symbol
	var walk func(hir.Pattern)
	walk = func(p hir.Pattern) {
		switch p := p.(type) {
		case *hir.ObjectPattern:
			for _, prop := range p.Properties {
				if prop == nil {
					continue
				}
				if prop.Value != nil {
					out = append(out, prop.Value)
				}
				if prop.Pattern != nil {
					walk(prop.Pattern)
				}
			}
			if p.Rest != nil {
				out = append(out, p.Rest)
			}
		case *hir.ArrayPattern:
			for _, elem := range p.Elements {
				if elem == nil {
					continue
				}
				if elem.Symbol != nil {
					out = append(out, elem.Symbol)
				}
				if elem.Pattern != nil {
					walk(elem.Pattern)
				}
			}
			if p.Rest != nil {
				out = append(out, p.Rest)
			}
		}
	}
	walk(pat)
	return out
}
func (l *Lowerer) lowerClassDecl(d *hir.ClassDecl) {
	l.jsvalueImport()
	name := l.emitName(d.Symbol)
	brandKey := l.nextSyntheticName(fmt.Sprintf("_brand_%s", symbol.Sanitize(name)))
	l.decls = append(l.decls, varDecl(brandKey, nil, l.brandKeyValue(name)))
	privateKeys := l.collectPrivateKeys(name, d.Properties, d.Methods)
	l.decls = append(l.decls, l.lowerPrivateKeyDecls(name, privateKeys)...)

	prevClassName := l.currentClassName
	prevClassBrand := l.currentClassBrand
	prevPrivateKeys := l.privateKeys
	l.currentClassName = name
	l.currentClassBrand = brandKey
	l.privateKeys = privateKeys
	defer func() {
		l.currentClassName = prevClassName
		l.currentClassBrand = prevClassBrand
		l.privateKeys = prevPrivateKeys
	}()

	var parentExpr ast.Expr = goIdent("nil")
	if d.Parent != nil {
		parentExpr = l.lowerExpr(d.Parent)
	}

	ctorLit := l.lowerClassConstructor(name, d.Parent != nil, d.Constructor, d.Properties, d.Methods)
	l.decls = append(l.decls, varDecl(name, nil,
		callExpr(selectorExpr(goIdent("jsvalue"), "NewClass"), ctorLit, parentExpr)))

	l.initStmts = append(l.initStmts, l.lowerClassSetups(goIdent(name), goIdent(brandKey), d.Properties, d.Methods, d.StaticInits)...)
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
func (l *Lowerer) brandKeyValue(className string) ast.Expr {
	l.jsvalueImport()
	desc := fmt.Sprintf("%s.#brand", className)
	return callExpr(
		selectorExpr(goIdent("jsvalue"), "PropertyKey"),
		callExpr(selectorExpr(goIdent("jsvalue"), "NewSymbol"), stringLit(desc)),
	)
}
func (l *Lowerer) privateBrandCheck(target ast.Expr) ast.Expr {
	if l.currentClassBrand == "" {
		return goIdent("false")
	}
	return callExpr(selectorExpr(target, "HasOwnProperty"), goIdent(l.currentClassBrand))
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

	var span *hir.SourceSpan
	if ctor != nil {
		span = ctor.Span
	}
	ctorBody = l.instrumentProfiledBody(className+".constructor", span, ctorBody)
	return setFuncLitPos(&ast.FuncLit{
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
	}, span)
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
	if l.currentClassBrand != "" {
		stmts = append(stmts, l.defineHiddenProperty(goIdent("this"), goIdent(l.currentClassBrand), callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent("true"))))
	}
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
func (l *Lowerer) lowerClassSetups(classRef ast.Expr, brandKey ast.Expr, props []*hir.ClassProperty, methods []*hir.ClassMethod, staticInits []hir.Expr) []ast.Stmt {
	var stmts []ast.Stmt
	if brandKey != nil {
		stmts = append(stmts, l.defineHiddenProperty(classRef, brandKey, callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent("true"))))
	}
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
		if method.IsGetter || method.IsSetter {
			l.jsvalueImport()
			var target ast.Expr = classRef
			if !method.IsStatic {
				target = callExpr(selectorExpr(classRef, "Get"), stringLit("prototype"))
			}
			var getter ast.Expr = goIdent("nil")
			var setter ast.Expr = goIdent("nil")
			methodVal := l.lowerClassMethodValue(method)
			if method.IsGetter {
				getter = methodVal
			}
			if method.IsSetter {
				setter = methodVal
			}
			stmts = append(stmts, exprStmt(callExpr(
				selectorExpr(goIdent("jsvalue"), "DefineAccessor"),
				target,
				l.lowerClassMemberKey(method.Name, method.IsPrivate, method.Computed),
				getter,
				setter,
			)))
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
	if m.IsAsync {
		if !m.IsStatic {
			methodBody = l.lowerAsyncFuncBody(m.Params, m.Body, 1, true)
		} else {
			methodBody = l.lowerAsyncFuncBody(m.Params, m.Body, 0, false)
		}
	} else if !m.IsStatic {
		methodBody = l.lowerMethodBody(m.Params, m.Body)
	} else {
		methodBody = l.lowerFuncBody(m.Params, m.Body)
	}
	methodBody = l.instrumentProfiledBody(m.Name, m.Span, methodBody)
	methodLit := setFuncLitPos(l.wrapAsJSValueFunc(m.Params, methodBody), m.Span)
	methodVal := l.generatedFunctionValue(m.Name, m.Span, methodLit)
	if m.IsAsync {
		methodVal = callExpr(selectorExpr(methodVal, "MarkAsAsync"))
	}
	methodVal = callExpr(selectorExpr(methodVal, "MarkAsMethod"))
	return methodVal
}
func (l *Lowerer) defineHiddenProperty(target, key, value ast.Expr) ast.Stmt {
	l.jsvalueImport()
	desc := callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"),
		stringLit("value"), value,
		stringLit("writable"), callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent("true")),
		stringLit("enumerable"), callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent("false")),
		stringLit("configurable"), callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent("true")),
	)
	return exprStmt(callExpr(selectorExpr(goIdent("jsvalue"), "DefineProperty"), target, key, desc))
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
			l.maybeEmitExportAlias(inner.Symbol, l.emitName(inner.Symbol))
		case *hir.VarDecl:
			for _, decl := range inner.Declarators {
				if decl.Symbol != nil {
					decl.Symbol.Exported = true
				}
			}
			l.lowerVarDecl(inner)
			for _, decl := range inner.Declarators {
				if decl.Symbol != nil {
					l.maybeEmitExportAlias(decl.Symbol, l.emitName(decl.Symbol))
				}
			}
		case *hir.ClassDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerClassDecl(inner)
			l.maybeEmitExportAlias(inner.Symbol, l.emitName(inner.Symbol))
		case *hir.EnumDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerEnumDecl(inner)
			l.maybeEmitExportAlias(inner.Symbol, l.emitName(inner.Symbol))
		case *hir.InterfaceDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerInterfaceDecl(inner)
			l.maybeEmitExportAlias(inner.Symbol, l.emitName(inner.Symbol))
		case *hir.TypeAliasDecl:
			if inner.Symbol != nil {
				inner.Symbol.Exported = true
			}
			l.lowerTypeAliasDecl(inner)
			l.maybeEmitExportAlias(inner.Symbol, l.emitName(inner.Symbol))
		default:
			l.lowerDecl(d.Decl)
		}
	}
	for _, n := range d.Names {
		goName := symbol.Capitalize(symbol.Sanitize(n.ExportedName))
		if alias, ok := l.exportAliasMap[n.ExportedName]; ok {
			goName = alias
		}
		if l.emittedExportNames[goName] {
			continue
		}
		var rhs ast.Expr = goIdent(symbol.Sanitize(n.LocalName))
		var sym *symbol.Symbol
		mappedFromModule := false
		if d.FromModule != "" {
			if mapped, ok := l.importNameMap[d.FromModule+"\x00"+n.LocalName]; ok {
				rhs = goIdent(mapped)
				mappedFromModule = true
			} else if mapped, ok := l.importNameMap[d.FromModule+"\x00*"]; ok && n.LocalName == "*" {
				rhs = goIdent(mapped)
				mappedFromModule = true
			} else if mapped, ok := l.importNameMap[d.FromModule+"\x00*"]; ok {
				rhs = callExpr(selectorExpr(goIdent(mapped), "Get"), stringLit(n.LocalName))
				mappedFromModule = true
			}
		}
		if lookedUp := l.symtab.Lookup(n.LocalName); lookedUp != nil {
			sym = lookedUp
			rhs = l.lowerIdentifier(&hir.Identifier{Sym: sym, Name: n.LocalName})
		} else if n.LocalName != "*" && !mappedFromModule {
			rhs = l.lowerIdentifier(&hir.Identifier{Name: n.LocalName})
		}
		if id, ok := rhs.(*ast.Ident); ok && id.Name == goName {
			continue
		}
		if sym != nil && sym.Kind == symbol.KindVariable && !l.eagerVarInits[sym.ID] {
			l.deferVarToInit(goName, rhs)
			continue
		}
		// Re-export aliases from other modules: defer to init() because
		// the referenced var may itself be set in init().
		if mappedFromModule {
			l.deferVarToInit(goName, rhs)
			continue
		}
		// Imported symbols (e.g. `import * as z; export { z }`) that resolve
		// to cross-file variables must also be deferred, since the target
		// variable may only be set in another file's init().
		if sym != nil && sym.Kind == symbol.KindImport {
			if _, isImported := l.importedSyms[sym]; isImported {
				l.deferVarToInit(goName, rhs)
				continue
			}
		}
		l.decls = append(l.decls, varDecl(goName, nil, rhs))
		l.emittedExportNames[goName] = true
	}
}
func (l *Lowerer) maybeEmitExportAlias(sym *symbol.Symbol, localGoName string) {
	if l.exportAliasMap == nil {
		return
	}
	if sym == nil {
		return
	}
	goName, ok := l.exportAliasMap[sym.OriginalName]
	if !ok || goName == "" || goName == localGoName {
		return
	}
	if sym.Kind == symbol.KindVariable && !l.eagerVarInits[sym.ID] {
		l.deferVarToInit(goName, goIdent(localGoName))
		return
	}
	l.decls = append(l.decls, varDecl(goName, nil, goIdent(localGoName)))
}

// deferVarToInit emits a forward declaration for goName and defers the
// assignment to init(). Used for exports that reference variables whose
// value may not be available at package level (cross-file references,
// re-exported aliases, etc.).
func (l *Lowerer) deferVarToInit(goName string, rhs ast.Expr) {
	if l.emittedExportNames[goName] {
		return
	}
	l.jsvalueImport()
	l.decls = append(l.decls, varDecl(goName, jsValuePtrType(), nil))
	l.initStmts = append(l.initStmts, assignStmt([]ast.Expr{goIdent(goName)}, []ast.Expr{rhs}))
	l.emittedExportNames[goName] = true
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
			defaultName := "Default"
			if alias, ok := l.exportAliasMap["default"]; ok {
				defaultName = alias
			}
			l.decls = append(l.decls, varDecl(defaultName, nil, goIdent(goName)))
		}
	case *hir.ClassDecl:
		if inner.Symbol != nil {
			inner.Symbol.Exported = true
		}
		l.lowerClassDecl(inner)
		if inner.Symbol != nil {
			localGoName := l.emitName(inner.Symbol)
			defaultName := "Default"
			if alias, ok := l.exportAliasMap["default"]; ok {
				defaultName = alias
			}
			if defaultName != localGoName {
				l.decls = append(l.decls, varDecl(defaultName, nil, goIdent(localGoName)))
			}
		}
	case *hir.VarDecl:
		// export default expr → var Default = expr
		if len(inner.Declarators) == 0 {
			return
		}
		decl := inner.Declarators[0]
		var value ast.Expr
		if decl.Init != nil {
			value = l.lowerExpr(decl.Init)
			value = jsvalueWrapLit(value)
		}
		defaultName := "Default"
		if alias, ok := l.exportAliasMap["default"]; ok {
			defaultName = alias
		}
		// If the default export references an imported symbol, defer to
		// init() since the target may only be set in another file's init().
		deferToInit := false
		if ident, ok := decl.Init.(*hir.Identifier); ok && ident.Sym != nil {
			if _, isImported := l.importedSyms[ident.Sym]; isImported {
				deferToInit = true
			}
		}
		if deferToInit {
			l.jsvalueImport()
			l.decls = append(l.decls, varDecl(defaultName, jsValuePtrType(), nil))
			l.initStmts = append(l.initStmts, assignStmt([]ast.Expr{goIdent(defaultName)}, []ast.Expr{value}))
		} else {
			l.decls = append(l.decls, varDecl(defaultName, nil, value))
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
	useAsJSValue := false
	if l.ctx != nil {
		if mod := l.ctx.LookupModule(d.ModulePath); mod != nil {
			overrides = mod.SymbolOverrides
			useAsJSValue = mod.UseAsJSValue
		}
	}

	// Process default import
	if d.Default != nil && d.Default.Symbol != nil {
		if mapped, ok := l.importNameMap[d.ModulePath+"\x00default"]; ok {
			res := importResolution{
				goImportPath: "",
				goPkgName:    "",
				goSymbol:     mapped,
				isTranspiled: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Default.Symbol] = res
			l.importedNames[d.Default.LocalName] = res
		} else if isKnown && useAsJSValue {
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				isTranspiled: false,
				useAsJSValue: true,
			}
			if overrides != nil {
				if ov, ok := overrides["default"]; ok && ov.GoSymbol != "" {
					res.goSymbol = ov.GoSymbol
					res.moduleValue = ov.GoSymbol
				}
			}
			if res.goSymbol == "PromisesAsJSValue" {
				res.jsExportName = ""
			}
			l.importedSyms[d.Default.Symbol] = res
			l.importedNames[d.Default.LocalName] = res
		} else if isKnown && isGunRuntimePkg(goImportPath) {
			// Legacy fallback: Default import from Gun runtime module → pkg.AsJSValue
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "AsJSValue",
				isTranspiled: false,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Default.Symbol] = res
			l.importedNames[d.Default.LocalName] = res
		} else if isKnown {
			// Default import from Go stdlib module → namespace (bare pkg ident)
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "",
				isTranspiled: false,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Default.Symbol] = res
			l.importedNames[d.Default.LocalName] = res
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
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     goSym,
				isTranspiled: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Default.Symbol] = res
			l.importedNames[d.Default.LocalName] = res
		}
	}

	// Process named imports
	for _, n := range d.Named {
		if n.Symbol == nil {
			continue
		}
		goSym := symbol.Capitalize(symbol.Sanitize(n.OriginalName))
		if mapped, ok := l.importNameMap[d.ModulePath+"\x00"+n.OriginalName]; ok {
			res := importResolution{
				goImportPath: "",
				goPkgName:    "",
				goSymbol:     mapped,
				isTranspiled: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[n.Symbol] = res
			l.importedNames[n.LocalName] = res
			continue
		}
		if useAsJSValue {
			moduleValue := ""
			if overrides != nil {
				if ov, ok := overrides["default"]; ok && ov.GoSymbol != "" {
					moduleValue = ov.GoSymbol
				}
			}
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				isTranspiled: false,
				useAsJSValue: true,
				moduleValue:  moduleValue,
				jsExportName: n.OriginalName,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[n.Symbol] = res
			l.importedNames[n.LocalName] = res
			continue
		}
		// Same-package imports: check if the symbol is exported from the other file
		// (and thus capitalized), or just an internal reference (stays lowercase).
		if goImportPath == "" {
			capName := symbol.Capitalize(symbol.Sanitize(n.OriginalName))
			if l.crossFileExports[capName] {
				goSym = capName // exported from other file → use capitalized
			} else {
				goSym = symbol.Sanitize(n.OriginalName) // not exported → use sanitized local name
			}
		}
		if overrides != nil {
			if ov, ok := overrides[n.OriginalName]; ok {
				goSym = ov.GoSymbol
			}
		}
		res := importResolution{
			goImportPath: goImportPath,
			goPkgName:    goPkgName,
			goSymbol:     goSym,
			isTranspiled: !isKnown,
			modulePath:   d.ModulePath,
		}
		l.importedSyms[n.Symbol] = res
		l.importedNames[n.LocalName] = res
	}

	// Process namespace import
	if d.Namespace != nil && d.Namespace.Symbol != nil {
		if mapped, ok := l.importNameMap[d.ModulePath+"\x00*"]; ok {
			res := importResolution{
				goImportPath: "",
				goPkgName:    "",
				goSymbol:     mapped,
				isTranspiled: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Namespace.Symbol] = res
			l.importedNames[d.Namespace.LocalName] = res
			return
		}
		if isKnown && useAsJSValue {
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				isTranspiled: false,
				useAsJSValue: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Namespace.Symbol] = res
			l.importedNames[d.Namespace.LocalName] = res
			return
		}
		if isKnown {
			// import * as fs from "fs" → fs.AsJSValue
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "AsJSValue",
				isTranspiled: false,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Namespace.Symbol] = res
			l.importedNames[d.Namespace.LocalName] = res
		} else {
			res := importResolution{
				goImportPath: goImportPath,
				goPkgName:    goPkgName,
				goSymbol:     "", // empty = namespace
				isTranspiled: true,
				modulePath:   d.ModulePath,
			}
			l.importedSyms[d.Namespace.Symbol] = res
			l.importedNames[d.Namespace.LocalName] = res
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
		clean = strings.TrimSuffix(clean, ".json")
		clean = strings.TrimSuffix(clean, ".yaml")
		clean = strings.TrimSuffix(clean, ".yml")
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
	name = strings.ReplaceAll(name, ".", "_")
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
	if l.hasTopLevelAwait && l.pkgName == "main" {
		l.topLevelAwaitStmts = append(l.topLevelAwaitStmts, d.Stmt)
		return
	}
	gs := l.lowerStmt(d.Stmt)
	if gs == nil {
		return
	}
	if l.pkgName == "main" {
		mainFn := l.getOrCreateMain()
		mainFn.Body.List = l.appendWithLineMarker(mainFn.Body.List, d.Span, gs)
	} else {
		// Non-main packages: top-level statements go into init()
		l.initStmts = l.appendWithLineMarker(l.initStmts, d.Span, gs)
	}
}

// getOrCreateMain returns the main() function declaration, creating it if needed.
func (l *Lowerer) getOrCreateMain() *ast.FuncDecl {
	// Look for existing main func
	for _, d := range l.decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "main" {
			l.ensureMainRecover(fd)
			return fd
		}
	}
	// Create one
	fd := funcDecl("main", fieldList(), nil, blockStmt())
	l.ensureMainRecover(fd)
	l.decls = append(l.decls, fd)
	return fd
}
func (l *Lowerer) ensureMainRecover(fd *ast.FuncDecl) {
	if fd == nil || fd.Name == nil || fd.Name.Name != "main" || fd.Body == nil {
		return
	}
	for _, stmt := range fd.Body.List {
		deferStmt, ok := stmt.(*ast.DeferStmt)
		if !ok || deferStmt.Call == nil {
			continue
		}
		if sel, ok := deferStmt.Call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "error" && sel.Sel.Name == "RecoverMain" {
				return
			}
		}
	}
	l.addAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
	fd.Body.List = append([]ast.Stmt{
		&ast.DeferStmt{Call: callExpr(selectorExpr(goIdent("error"), "RecoverMain"))},
	}, fd.Body.List...)
}
func (l *Lowerer) injectCPUProfileMain() {
	if l.cpuProfile == nil || l.pkgName != "main" {
		return
	}
	mainFn := l.findMainFunc()
	if mainFn == nil || mainFn.Body == nil {
		return
	}

	profileAlias := l.ensureProfileRuntimeAlias()
	stopIdent := goIdent("_gunCPUProfileStop")
	startCall := callExpr(
		selectorExpr(goIdent(profileAlias), "StartCPUProfileOrExit"),
		stringLit(l.cpuProfile.Dir),
		stringLit(l.cpuProfile.Name),
	)
	startStmt := assignDefine([]ast.Expr{stopIdent}, []ast.Expr{startCall})
	deferStmt := &ast.DeferStmt{Call: callExpr(stopIdent)}

	insertAt := 0
	if len(mainFn.Body.List) > 0 {
		if deferStmtExisting, ok := mainFn.Body.List[0].(*ast.DeferStmt); ok && deferStmtExisting.Call != nil {
			if sel, ok := deferStmtExisting.Call.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "error" && sel.Sel.Name == "RecoverMain" {
					insertAt = 1
				}
			}
		}
	}

	stmts := []ast.Stmt{startStmt, deferStmt}
	body := append([]ast.Stmt{}, mainFn.Body.List[:insertAt]...)
	body = append(body, stmts...)
	mainFn.Body.List = append(body, mainFn.Body.List[insertAt:]...)
}
func (l *Lowerer) uniqueInternalImportAlias(base string) string {
	candidate := base
	for suffix := 1; l.internalImportAliasTaken(candidate); suffix++ {
		candidate = fmt.Sprintf("%s_%d", base, suffix)
	}
	l.symtab.ReserveNameStr(candidate)
	return candidate
}
func (l *Lowerer) internalImportAliasTaken(alias string) bool {
	if l.crossFileExports[alias] || l.reservedNames[alias] {
		return true
	}
	for _, emitted := range l.topLevelNames {
		if emitted == alias {
			return true
		}
	}
	for _, importAlias := range l.imports {
		if importAlias == alias {
			return true
		}
	}
	return false
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
		case *hir.VarDecl:
			for _, decl := range d.Declarators {
				l.registerTopLevelName(decl.Symbol)
			}
			for _, decl := range d.Declarators {
				if decl.Symbol != nil && decl.Init != nil && !hirExprReferencesName(decl.Init, decl.Symbol.OriginalName) {
					l.eagerVarInits[decl.Symbol.ID] = true
				}
			}
		case *hir.FuncDecl:
			l.registerTopLevelName(d.Symbol)
			if d.Symbol != nil && d.Symbol.FuncInfo != nil {
				// Already captured by HIR builder
			}
		case *hir.ClassDecl:
			l.registerTopLevelName(d.Symbol)
		case *hir.EnumDecl:
			l.registerTopLevelName(d.Symbol)
		case *hir.InterfaceDecl:
			l.registerTopLevelName(d.Symbol)
		case *hir.TypeAliasDecl:
			l.registerTopLevelName(d.Symbol)
		case *hir.ExportDecl:
			if d.Decl != nil {
				if fd, ok := d.Decl.(*hir.FuncDecl); ok && fd.Symbol != nil {
					fd.Symbol.Exported = true
					l.registerTopLevelName(fd.Symbol)
				}
				if vd, ok := d.Decl.(*hir.VarDecl); ok {
					for _, decl := range vd.Declarators {
						if decl.Symbol != nil {
							decl.Symbol.Exported = true
							l.registerTopLevelName(decl.Symbol)
							if decl.Init != nil && !hirExprReferencesName(decl.Init, decl.Symbol.OriginalName) {
								l.eagerVarInits[decl.Symbol.ID] = true
							}
						}
					}
				}
				if cd, ok := d.Decl.(*hir.ClassDecl); ok && cd.Symbol != nil {
					cd.Symbol.Exported = true
					l.registerTopLevelName(cd.Symbol)
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
				capName := symbol.Capitalize(symbol.Sanitize(d.Symbol.OriginalName))
				// Only mark as exported if the capitalized name is in cross-file exports
				// AND no other local symbol already claims that capitalized name
				if l.crossFileExports[capName] && !localNames[capName] {
					d.Symbol.Exported = true
				}
			}
		case *hir.VarDecl:
			for _, decl := range d.Declarators {
				if decl.Symbol != nil {
					capName := symbol.Capitalize(symbol.Sanitize(decl.Symbol.OriginalName))
					if l.crossFileExports[capName] && !localNames[capName] {
						decl.Symbol.Exported = true
					}
				}
			}
		case *hir.ClassDecl:
			if d.Symbol != nil {
				capName := symbol.Capitalize(symbol.Sanitize(d.Symbol.OriginalName))
				if l.crossFileExports[capName] && !localNames[capName] {
					d.Symbol.Exported = true
				}
			}
		}
	}
}

// fixInitCycles detects package-level variable initializer cycles
// and splits the participating vars into forward declarations + init() assignments.
func (l *Lowerer) fixInitCycles(decls []ast.Decl) []ast.Decl {
	type varInfo struct {
		name  string
		typ   ast.Expr
		value ast.Expr
		decl  *ast.GenDecl
	}

	varInfos := make(map[string]varInfo)
	for _, d := range decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 {
				continue
			}
			info := varInfo{name: vs.Names[0].Name, typ: vs.Type, decl: gd}
			if len(vs.Values) > 0 {
				info.value = vs.Values[0]
			}
			varInfos[info.name] = info
		}
	}

	deps := make(map[string]map[string]bool)
	for name, info := range varInfos {
		if info.value == nil {
			continue
		}
		refs := exprReferencedIdents(info.value)
		for ref := range refs {
			if ref == name {
				if deps[name] == nil {
					deps[name] = make(map[string]bool)
				}
				deps[name][ref] = true
				continue
			}
			if _, ok := varInfos[ref]; ok {
				if deps[name] == nil {
					deps[name] = make(map[string]bool)
				}
				deps[name][ref] = true
			}
		}
	}

	cyclic := make(map[string]bool)
	visiting := make(map[string]bool)
	memo := make(map[string]bool)
	var inCycle func(string) bool
	inCycle = func(name string) bool {
		if v, ok := memo[name]; ok {
			return v
		}
		if visiting[name] {
			return true
		}
		visiting[name] = true
		defer delete(visiting, name)
		for dep := range deps[name] {
			if dep == name || inCycle(dep) {
				memo[name] = true
				return true
			}
		}
		memo[name] = false
		return false
	}
	for name := range varInfos {
		if inCycle(name) {
			cyclic[name] = true
		}
	}

	var result []ast.Decl
	var initStmts []ast.Stmt

	for _, d := range decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			result = append(result, d)
			continue
		}
		var keptSpecs []ast.Spec
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 {
				continue
			}
			name := vs.Names[0].Name
			if !cyclic[name] {
				keptSpecs = append(keptSpecs, spec)
				continue
			}
			l.jsvalueImport()
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
		if len(keptSpecs) > 0 {
			result = append(result, &ast.GenDecl{Tok: token.VAR, Specs: keptSpecs})
		}
	}

	if len(initStmts) > 0 {
		initFn := funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: initStmts})
		result = append(result, initFn)
	}

	return result
}
func exprReferencedIdents(expr ast.Expr) map[string]bool {
	refs := make(map[string]bool)
	var stack []ast.Node
	ast.Inspect(expr, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		if id, ok := n.(*ast.Ident); ok {
			if sel, ok := parent.(*ast.SelectorExpr); ok && sel.Sel == id {
				stack = append(stack, n)
				return true
			}
			refs[id.Name] = true
		}
		stack = append(stack, n)
		return true
	})
	return refs
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

	if l.arenaEnabled && hirBodyNeedsArena(body) && !hirBodyContainsNestedClosure(body) {
		l.jsvalueImport()
		l.hasArenaVar++
		defer func() { l.hasArenaVar-- }()
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl("_arena", ptrType(selectorExpr(goIdent("jsvalue"), "Arena")), nil)})
		stmts = append(stmts, assignStmt([]ast.Expr{goIdent("_arena")}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "GetArena"))}))
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(goIdent("_arena"), "PushScope"))))
		stmts = append(stmts, &ast.DeferStmt{Call: callExpr(selectorExpr(goIdent("jsvalue"), "ReleaseArena"), goIdent("_arena"))})
		stmts = append(stmts, &ast.DeferStmt{Call: callExpr(selectorExpr(goIdent("_arena"), "PopScope"))})
	}

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
					Cond: l.isNilOrUndefined(goIdent(tmpName)),
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
					Cond: l.isNilOrUndefined(goIdent(name)),
					Body: blockStmt(assignStmt(
						[]ast.Expr{goIdent(name)},
						[]ast.Expr{defVal},
					)),
				})
			}
		}
	}

	// Hoist function declarations within the body, but keep param unpacking/setup
	// statements ahead of any hoisted assignments.
	setupBarrier := len(stmts)
	// Hoist function declarations to top of body (JS hoisting semantics)
	hoistedBody := hoistFunctions(body.Stmts)

	// Lower body statements, flattening inline blocks (multi-declarator VarDecl)
	for _, s := range hoistedBody {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		span := hirStmtSpan(s)
		if block, ok := gs.(*ast.BlockStmt); ok {
			if true {
				for _, child := range block.List {
					stmts = l.appendWithLineMarker(stmts, span, child)
				}
				continue
			}
		}
		stmts = l.appendWithLineMarker(stmts, span, gs)
	}

	// Ensure trailing return nil if function body doesn't end with a return
	if !endsWithReturn(stmts) {
		stmts = append(stmts, returnStmt(goIdent("nil")))
	}

	// Forward-declare variables used before their := definition
	stmts = forwardDeclareVars(stmts, setupBarrier)

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
	if l.arenaEnabled && hirBodyNeedsArena(body) && !hirBodyContainsNestedClosure(body) {
		l.hasArenaVar++
		defer func() { l.hasArenaVar-- }()
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl("_arena", ptrType(selectorExpr(goIdent("jsvalue"), "Arena")), nil)})
		stmts = append(stmts, assignStmt([]ast.Expr{goIdent("_arena")}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "GetArena"))}))
		stmts = append(stmts, exprStmt(callExpr(selectorExpr(goIdent("_arena"), "PushScope"))))
		stmts = append(stmts, &ast.DeferStmt{Call: callExpr(selectorExpr(goIdent("jsvalue"), "ReleaseArena"), goIdent("_arena"))})
		stmts = append(stmts, &ast.DeferStmt{Call: callExpr(selectorExpr(goIdent("_arena"), "PopScope"))})
	}

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
				Cond: l.isNilOrUndefined(goIdent(name)),
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(name)},
					[]ast.Expr{defVal},
				)),
			})
		}
	}

	// Keep receiver/parameter setup statements ahead of any hoisted assignments.
	setupBarrier := len(stmts)
	// Lower body statements, flattening inline blocks
	for _, s := range body.Stmts {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		span := hirStmtSpan(s)
		if block, ok := gs.(*ast.BlockStmt); ok {
			if true {
				for _, child := range block.List {
					stmts = l.appendWithLineMarker(stmts, span, child)
				}
				continue
			}
		}
		stmts = l.appendWithLineMarker(stmts, span, gs)
	}

	// Ensure trailing return nil
	if !endsWithReturn(stmts) {
		stmts = append(stmts, returnStmt(goIdent("nil")))
	}

	// Forward-declare variables used before their := definition
	stmts = forwardDeclareVars(stmts, setupBarrier)

	// Replace unused := variables with _ to satisfy Go's "declared and not used" rule
	stmts = eliminateUnusedVars(stmts)

	return &ast.BlockStmt{List: stmts}
}

// forwardDeclareVars scans Go statements for variables used before their :=
// declaration. Adds `var name *jsvalue.JSValue` at top and changes := to =.
// Also hoists function-valued assignments to the top (JS function hoisting).
func forwardDeclareVars(stmts []ast.Stmt, hoistBarrier int) []ast.Stmt {
	if hoistBarrier < 0 {
		hoistBarrier = 0
	}
	if hoistBarrier > len(stmts) {
		hoistBarrier = len(stmts)
	}
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
	// Self-referential function literals also need forward declarations because
	// Go does not put a := variable in scope on its own RHS.
	for _, s := range stmts {
		assign, ok := s.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || !isFuncValuedAssign(assign) {
			continue
		}
		for _, lhs := range assign.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok {
				continue
			}
			if assignRHSReferencesIdent(assign, id.Name) {
				forwarded[id.Name] = true
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
		prefixDeclared := make(map[string]bool)
		for i := 0; i < hoistBarrier && i < len(stmts); i++ {
			inspectWithoutNestedFuncLits(stmts[i], func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range n.Lhs {
						if id, ok := lhs.(*ast.Ident); ok {
							prefixDeclared[id.Name] = true
						}
					}
				case *ast.GenDecl:
					if n.Tok != token.VAR {
						return true
					}
					for _, spec := range n.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range vs.Names {
								prefixDeclared[name.Name] = true
							}
						}
					}
				}
				return true
			})
		}
		// Hoisted function bodies may reference variables (argv, flags, etc.)
		// that are assigned later. These variables need var declarations before
		// the hoisted code. Scan hoisted functions for referenced names and
		// add var decls for any that aren't already forward-declared.
		existingDecls := make(map[string]bool)
		for name := range forwarded {
			existingDecls[name] = true
		}
		// Find all later local names in remaining statements, including bare
		// var declarations and nested assignments inside control flow.
		assignedNames := make(map[string]bool)
		assignedTypes := make(map[string]ast.Expr)
		for _, s := range remaining {
			inspectWithoutNestedFuncLits(s, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range n.Lhs {
						if id, ok := lhs.(*ast.Ident); ok {
							assignedNames[id.Name] = true
							if _, exists := assignedTypes[id.Name]; !exists {
								assignedTypes[id.Name] = jsValuePtrType()
							}
						}
					}
				case *ast.GenDecl:
					if n.Tok != token.VAR {
						return true
					}
					for _, spec := range n.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range vs.Names {
								assignedNames[name.Name] = true
								if _, exists := assignedTypes[name.Name]; !exists {
									if vs.Type != nil {
										assignedTypes[name.Name] = vs.Type
									} else {
										assignedTypes[name.Name] = jsValuePtrType()
									}
								}
							}
						}
					}
				}
				return true
			})
		}
		// Check which names hoisted functions reference that aren't declared yet
		var extraDecls []ast.Stmt
		for _, s := range hoisted {
			ast.Inspect(s, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && !existingDecls[id.Name] && !prefixDeclared[id.Name] && assignedNames[id.Name] {
					existingDecls[id.Name] = true
					forwarded[id.Name] = true
					declType := assignedTypes[id.Name]
					if declType == nil {
						declType = jsValuePtrType()
					}
					extraDecls = append(extraDecls, &ast.DeclStmt{Decl: varDecl(id.Name, declType, nil)})
					// Change the corresponding := in remaining to = since we forward-declared it
					for _, rs := range remaining {
						inspectWithoutNestedFuncLits(rs, func(n ast.Node) bool {
							assign, ok := n.(*ast.AssignStmt)
							if !ok || assign.Tok != token.DEFINE {
								return true
							}
							for _, lhs := range assign.Lhs {
								if lid, ok := lhs.(*ast.Ident); ok && lid.Name == id.Name {
									assign.Tok = token.ASSIGN
								}
							}
							return true
						})
					}
				}
				return true
			})
		}
		// Remove original bare var declarations for any names newly forwarded
		// while scanning hoisted function captures.
		var refiltered []ast.Stmt
		for _, s := range remaining {
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
			refiltered = append(refiltered, s)
		}
		remaining = refiltered
		fwd = append(fwd, extraDecls...)
		var prefix []ast.Stmt
		var post []ast.Stmt
		for i, s := range remaining {
			if i < hoistBarrier {
				prefix = append(prefix, s)
			} else {
				post = append(post, s)
			}
		}
		out := append([]ast.Stmt{}, fwd...)
		out = append(out, prefix...)
		out = append(out, hoisted...)
		out = append(out, post...)
		return out
	}

	return append(fwd, stmts...)
}
func inspectWithoutNestedFuncLits(root ast.Node, fn func(ast.Node) bool) {
	first := true
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			return fn(n)
		}
		if _, ok := n.(*ast.FuncLit); ok && !first {
			return false
		}
		first = false
		return fn(n)
	})
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
			return id.Name == "jsvalue" && (sel.Sel.Name == "NewFunction" || sel.Sel.Name == "NewArenaFunction")
		}
	}
	return false
}
func assignRHSReferencesIdent(assign *ast.AssignStmt, name string) bool {
	for _, rhs := range assign.Rhs {
		found := false
		ast.Inspect(rhs, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == name {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
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
func hirBodyNeedsArena(body *hir.BlockStmt) bool {
	if body == nil {
		return false
	}
	needsArena := false
	var walkExpr func(e hir.Expr)
	var walkStmt func(s hir.Stmt)
	walkExpr = func(e hir.Expr) {
		if needsArena || e == nil {
			return
		}
		switch e := e.(type) {
		case *hir.BinaryExpr:
			switch e.Op {
			case hir.OpAdd, hir.OpSub, hir.OpMul, hir.OpDiv, hir.OpMod:
				needsArena = true
				return
			}
			walkExpr(e.Left)
			walkExpr(e.Right)
		case *hir.UnaryExpr:
			if e.Op == hir.OpNeg {
				needsArena = true
				return
			}
			walkExpr(e.Operand)
		case *hir.UpdateExpr:
			walkExpr(e.Operand)
		case *hir.AssignExpr:
			walkExpr(e.Left)
			walkExpr(e.Right)
		case *hir.CallExpr:
			walkExpr(e.Func)
			for _, a := range e.Args {
				walkExpr(a)
			}
		case *hir.NewExpr:
			walkExpr(e.Callee)
			for _, a := range e.Args {
				walkExpr(a)
			}
		case *hir.MemberExpr:
			walkExpr(e.Object)
		case *hir.ComputedMemberExpr:
			walkExpr(e.Object)
			walkExpr(e.Property)
		case *hir.TernaryExpr:
			walkExpr(e.Cond)
			walkExpr(e.Then)
			walkExpr(e.Else)
		case *hir.ArrayLiteral:
			for _, el := range e.Elements {
				walkExpr(el)
			}
		case *hir.ObjectLiteral:
			for _, p := range e.Properties {
				if p.Computed {
					walkExpr(p.Key)
				}
				walkExpr(p.Value)
			}
		case *hir.TemplateLiteral:
			for _, p := range e.Parts {
				walkExpr(p)
			}
		case *hir.TaggedTemplateLiteral:
			walkExpr(e.Tag)
			if e.Template != nil {
				for _, p := range e.Template.Parts {
					walkExpr(p)
				}
			}
		case *hir.SequenceExpr:
			for _, ex := range e.Exprs {
				walkExpr(ex)
			}
		case *hir.SpreadExpr:
			walkExpr(e.Value)
		case *hir.AwaitExpr:
			walkExpr(e.Value)
		case *hir.YieldExpr:
			walkExpr(e.Value)
		case *hir.TypeAssertExpr:
			walkExpr(e.Expr)
		case *hir.NonNullExpr:
			walkExpr(e.Expr)
		case *hir.ParenExpr:
			walkExpr(e.Expr)
		}
	}
	walkStmt = func(s hir.Stmt) {
		if needsArena || s == nil {
			return
		}
		switch s := s.(type) {
		case *hir.ExprStmt:
			walkExpr(s.Expr)
		case *hir.ReturnStmt:
			walkExpr(s.Value)
		case *hir.IfStmt:
			walkExpr(s.Cond)
			walkStmt(s.Then)
			walkStmt(s.Else)
		case *hir.ForStmt:
			walkStmt(s.Init)
			walkExpr(s.Cond)
			walkExpr(s.Post)
			walkStmt(s.Body)
		case *hir.ForOfStmt:
			walkExpr(s.Value)
			walkStmt(s.Body)
		case *hir.ForInStmt:
			walkExpr(s.Value)
			walkStmt(s.Body)
		case *hir.WhileStmt:
			walkExpr(s.Cond)
			walkStmt(s.Body)
		case *hir.DoWhileStmt:
			walkStmt(s.Body)
			walkExpr(s.Cond)
		case *hir.SwitchStmt:
			walkExpr(s.Tag)
			for _, c := range s.Cases {
				walkExpr(c.Value)
				for _, st := range c.Body {
					walkStmt(st)
				}
			}
		case *hir.TryCatchStmt:
			walkStmt(s.Try)
			if s.Catch != nil {
				walkStmt(s.Catch.Body)
			}
			walkStmt(s.Finally)
		case *hir.BlockStmt:
			if s == nil {
				return
			}
			for _, st := range s.Stmts {
				walkStmt(st)
			}
		case *hir.VarDecl:
			for _, d := range s.Declarators {
				walkExpr(d.Init)
			}
		case *hir.ThrowStmt:
			walkExpr(s.Value)
		case *hir.LabeledStmt:
			walkStmt(s.Stmt)
		}
	}
	walkStmt(body)
	return needsArena
}
func hirBodyContainsNestedClosure(body *hir.BlockStmt) bool {
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
		case *hir.ArrowFunc, *hir.FuncExpr:
			found = true
			return
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
				if p.Computed {
					walk(p.Key)
				}
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
		case *hir.AwaitExpr:
			walk(e.Value)
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
		case *hir.TypeAssertExpr:
			walk(e.Expr)
		case *hir.NonNullExpr:
			walk(e.Expr)
		case *hir.TaggedTemplateLiteral:
			walk(e.Tag)
			if e.Template != nil {
				for _, p := range e.Template.Parts {
					walk(p)
				}
			}
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
		case *hir.IfStmt:
			walk(s.Cond)
			for _, st := range s.Then.Stmts {
				walkStmt(st)
			}
			walkStmt(s.Else)
		case *hir.ForStmt:
			walkStmt(s.Init)
			walk(s.Cond)
			walk(s.Post)
			for _, st := range s.Body.Stmts {
				walkStmt(st)
			}
		case *hir.ForOfStmt:
			walk(s.Value)
			for _, st := range s.Body.Stmts {
				walkStmt(st)
			}
		case *hir.ForInStmt:
			walk(s.Value)
			for _, st := range s.Body.Stmts {
				walkStmt(st)
			}
		case *hir.WhileStmt:
			walk(s.Cond)
			for _, st := range s.Body.Stmts {
				walkStmt(st)
			}
		case *hir.DoWhileStmt:
			for _, st := range s.Body.Stmts {
				walkStmt(st)
			}
			walk(s.Cond)
		case *hir.SwitchStmt:
			walk(s.Tag)
			for _, c := range s.Cases {
				walk(c.Value)
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
			if s.Finally != nil {
				for _, st := range s.Finally.Stmts {
					walkStmt(st)
				}
			}
		case *hir.BlockStmt:
			for _, st := range s.Stmts {
				walkStmt(st)
			}
		case *hir.VarDecl:
			for _, d := range s.Declarators {
				walk(d.Init)
			}
		case *hir.ThrowStmt:
			walk(s.Value)
		case *hir.LabeledStmt:
			walkStmt(s.Stmt)
		}
	}
	for _, st := range body.Stmts {
		walkStmt(st)
	}
	return found
}
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
		case *hir.AwaitExpr:
			walk(e.Value)
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
			// Regular function expressions have their own `this`.
			// Do not treat nested function bodies as outer-body `this` usage.
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
func hirExprReferencesName(expr hir.Expr, name string) bool {
	found := false
	var walk func(hir.Expr)
	walk = func(e hir.Expr) {
		if found || e == nil {
			return
		}
		switch e := e.(type) {
		case *hir.Identifier:
			if e.Name == name || (e.Sym != nil && e.Sym.OriginalName == name) {
				found = true
			}
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
			if e.ExprBody != nil {
				walk(e.ExprBody)
			}
			if e.Body != nil {
				for _, st := range e.Body.Stmts {
					walkStmtExprRefs(st, name, &found)
					if found {
						return
					}
				}
			}
		case *hir.FuncExpr:
			if e.Body != nil {
				for _, st := range e.Body.Stmts {
					walkStmtExprRefs(st, name, &found)
					if found {
						return
					}
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
		case *hir.AwaitExpr:
			walk(e.Value)
		}
	}
	walk(expr)
	return found
}
func walkStmtExprRefs(stmt hir.Stmt, name string, found *bool) {
	if stmt == nil || *found {
		return
	}
	switch s := stmt.(type) {
	case *hir.ExprStmt:
		if hirExprReferencesName(s.Expr, name) {
			*found = true
		}
	case *hir.ReturnStmt:
		if hirExprReferencesName(s.Value, name) {
			*found = true
		}
	case *hir.VarDecl:
		for _, d := range s.Declarators {
			if hirExprReferencesName(d.Init, name) {
				*found = true
				return
			}
		}
	case *hir.IfStmt:
		if hirExprReferencesName(s.Cond, name) {
			*found = true
			return
		}
		for _, st := range s.Then.Stmts {
			walkStmtExprRefs(st, name, found)
			if *found {
				return
			}
		}
		walkStmtExprRefs(s.Else, name, found)
	case *hir.ForStmt:
		walkStmtExprRefs(s.Init, name, found)
		if *found || hirExprReferencesName(s.Cond, name) || hirExprReferencesName(s.Post, name) {
			*found = true
			return
		}
		for _, st := range s.Body.Stmts {
			walkStmtExprRefs(st, name, found)
			if *found {
				return
			}
		}
	case *hir.BlockStmt:
		for _, st := range s.Stmts {
			walkStmtExprRefs(st, name, found)
			if *found {
				return
			}
		}
	case *hir.SwitchStmt:
		if hirExprReferencesName(s.Tag, name) {
			*found = true
			return
		}
		for _, c := range s.Cases {
			for _, st := range c.Body {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.TryCatchStmt:
		if s.Try != nil {
			for _, st := range s.Try.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
		if s.Catch != nil && s.Catch.Body != nil {
			for _, st := range s.Catch.Body.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
		if s.Finally != nil {
			for _, st := range s.Finally.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.ThrowStmt:
		if hirExprReferencesName(s.Value, name) {
			*found = true
		}
	case *hir.WhileStmt:
		if hirExprReferencesName(s.Cond, name) {
			*found = true
			return
		}
		if s.Body != nil {
			for _, st := range s.Body.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.DoWhileStmt:
		if hirExprReferencesName(s.Cond, name) {
			*found = true
			return
		}
		if s.Body != nil {
			for _, st := range s.Body.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.ForInStmt:
		if hirExprReferencesName(s.Value, name) {
			*found = true
			return
		}
		if s.Body != nil {
			for _, st := range s.Body.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.ForOfStmt:
		if hirExprReferencesName(s.Value, name) {
			*found = true
			return
		}
		if s.Body != nil {
			for _, st := range s.Body.Stmts {
				walkStmtExprRefs(st, name, found)
				if *found {
					return
				}
			}
		}
	case *hir.LabeledStmt:
		walkStmtExprRefs(s.Stmt, name, found)
	}
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
func (l *Lowerer) generatedFunctionCtorName(fnLit *ast.FuncLit) string {
	if l.arenaEnabled && l.disableArenaCount == 0 && funcLitUsesArena(fnLit) {
		return "NewArenaFunction"
	}
	return "NewFunction"
}
func (l *Lowerer) generatedFunctionValue(name string, span *hir.SourceSpan, fnLit *ast.FuncLit) ast.Expr {
	return callExpr(selectorExpr(goIdent("jsvalue"), l.generatedFunctionCtorName(fnLit)), fnLit)
}
func funcLitUsesArena(fnLit *ast.FuncLit) bool {
	if fnLit == nil || fnLit.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fnLit.Body, func(n ast.Node) bool {
		if found || n == nil {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == "_arena" {
			found = true
			return false
		}
		return true
	})
	return found
}
func (l *Lowerer) instrumentProfiledBody(name string, span *hir.SourceSpan, body *ast.BlockStmt) *ast.BlockStmt {
	if l.cpuProfile == nil || body == nil {
		return body
	}
	l.ensureProfileRuntimeAlias()
	leaveName := l.nextSyntheticName("_gunProfileLeave")
	frameName := name
	if frameName == "" {
		frameName = "(anonymous)"
	}
	line := 0
	column := 0
	if span != nil {
		line = span.StartLine
		column = span.StartColumn
	}
	enter := assignDefine(
		[]ast.Expr{goIdent(leaveName)},
		[]ast.Expr{callExpr(selectorExpr(goIdent(l.profileRuntimeAlias), "EnterFrame"), callExpr(selectorExpr(goIdent(l.profileRuntimeAlias), "Frame"),
			stringLit(frameName),
			stringLit(l.sourcePath),
			intLit(itoa(line)),
			intLit(itoa(column)),
		))},
	)
	// Cannot construct struct literal with helper callExpr; use composite literal directly.
	enter.Rhs[0] = callExpr(selectorExpr(goIdent(l.profileRuntimeAlias), "EnterFrame"), &ast.CompositeLit{
		Type: selectorExpr(goIdent(l.profileRuntimeAlias), "Frame"),
		Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: goIdent("FunctionName"), Value: stringLit(frameName)},
			&ast.KeyValueExpr{Key: goIdent("File"), Value: stringLit(l.sourcePath)},
			&ast.KeyValueExpr{Key: goIdent("Line"), Value: intLit(itoa(line))},
			&ast.KeyValueExpr{Key: goIdent("Column"), Value: intLit(itoa(column))},
		},
	})
	deferStmt := &ast.DeferStmt{Call: callExpr(goIdent(leaveName))}
	body.List = append([]ast.Stmt{enter, deferStmt}, body.List...)
	return body
}
func (l *Lowerer) ensureProfileRuntimeAlias() string {
	if l.profileRuntimeAlias == "" {
		l.profileRuntimeAlias = l.uniqueInternalImportAlias("_gunprofile_runtime")
		l.addAliasedImport("github.com/nnstd/gun/runtime/profile", l.profileRuntimeAlias)
	}
	return l.profileRuntimeAlias

}
