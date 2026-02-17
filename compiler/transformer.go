package compiler

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// globalBuiltins is a shared builtins registry used by package-level functions.
var globalBuiltins = NewBuiltinRegistry()

// Transformer walks a tree-sitter TypeScript AST and builds a go/ast.File.
type Transformer struct {
	source             []byte
	pkgName            string
	moduleName         string                     // Go module name from go.mod (for relative imports)
	samePackageImports bool                        // treat relative imports as same-package refs
	decls              []ast.Decl
	imports            map[string]string          // Go import path → alias (empty = no alias)
	importedNames      map[string]resolvedImport  // TS name → Go resolution
	varTypes           map[string]string          // variable name → module type (e.g. "app" → "hono")
	funcVarNames       map[string]bool            // package-level vars assigned function literals (can't have Go fields)
	localScopes        []map[string]bool          // stack of local variable/parameter names that shadow imports (true = has type annotation, false = JSValue default)
	jsvalueLocals      map[string]bool            // local variables that hold *jsvalue.JSValue (not slices or maps)
	jsvalueSliceLocals map[string]bool            // typed locals whose elements are *jsvalue.JSValue (e.g. []*jsvalue.JSValue slices)
	mapLocals          map[string]bool            // typed locals initialized from object literals (map[string]*jsvalue.JSValue)
	typedLocalTypes    map[string]string          // typed local name → Go type name (e.g. "bool", "string", "[]string")
	pkgVarTyped        map[string]bool            // package-level variable name → true if typed (not JSValue)
	funcParamCounts    map[string]int             // hoisted function name → parameter count (for padding missing args)
	funcReturnTypes    map[string]string          // function name → Go return type (e.g. "bool", "*jsvalue.JSValue")
	builtins           *BuiltinRegistry           // registry of built-in methods and their metadata
}

func newTransformer(source []byte, pkgName, moduleName string, samePackageImports bool) *Transformer {
	return &Transformer{
		source:             source,
		pkgName:            pkgName,
		moduleName:         moduleName,
		samePackageImports: samePackageImports,
		imports:            make(map[string]string),
		importedNames:      make(map[string]resolvedImport),
		varTypes:           make(map[string]string),
		funcVarNames:       make(map[string]bool),
		jsvalueLocals:      make(map[string]bool),
		jsvalueSliceLocals: make(map[string]bool),
		mapLocals:          make(map[string]bool),
		typedLocalTypes:    make(map[string]string),
		pkgVarTyped:        make(map[string]bool),
		funcParamCounts:    make(map[string]int),
		funcReturnTypes:    make(map[string]string),
		builtins:           NewBuiltinRegistry(),
	}
}

func (t *Transformer) transform(root *sitter.Node) *ast.File {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		t.transformTopLevel(child)
	}

	// Ensure main() exists for runnable packages
	if t.pkgName == "main" {
		t.getOrCreateMain()
	}

	file := &ast.File{
		Name:  ident(t.pkgName),
		Decls: t.decls,
	}

	// Add imports
	if len(t.imports) > 0 {
		var specs []ast.Spec
		for pkg, alias := range t.imports {
			specs = append(specs, importSpecAlias(pkg, alias))
		}
		importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: specs}
		if len(specs) > 1 {
			importDecl.Lparen = 1 // triggers parenthesized form
		}
		file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
	}

	return file
}

func (t *Transformer) addImport(pkg string) {
	if _, ok := t.imports[pkg]; !ok {
		t.imports[pkg] = ""
	}
}

func (t *Transformer) addAliasedImport(pkg, alias string) {
	t.imports[pkg] = alias
}

// pushScope starts a new local scope (e.g. when entering a function body).
// The names slice contains parameter/variable names. Use pushTypedScope to
// also record which names have explicit type annotations.
func (t *Transformer) pushScope(names []string) {
	scope := make(map[string]bool, len(names))
	for _, n := range names {
		scope[n] = false // false = no type annotation (JSValue default)
	}
	t.localScopes = append(t.localScopes, scope)
}

// pushTypedScope starts a new local scope with type annotation info.
// The map values indicate whether each name has an explicit type annotation
// (true = typed, false = JSValue default).
func (t *Transformer) pushTypedScope(names map[string]bool) {
	t.localScopes = append(t.localScopes, names)
}

// popScope removes the most recent local scope.
func (t *Transformer) popScope() {
	if len(t.localScopes) > 0 {
		t.localScopes = t.localScopes[:len(t.localScopes)-1]
	}
}

// isLocalName returns true if the name is shadowed by a parameter/local in any active scope.
func (t *Transformer) isLocalName(name string) bool {
	for i := len(t.localScopes) - 1; i >= 0; i-- {
		if _, ok := t.localScopes[i][name]; ok {
			return true
		}
	}
	return false
}

// addToCurrentScope registers a variable name in the current (innermost) scope.
// typed indicates whether the variable has an explicit type annotation.
func (t *Transformer) addToCurrentScope(name string, typed bool) {
	if len(t.localScopes) > 0 {
		t.localScopes[len(t.localScopes)-1][name] = typed
	}
}

// isPkgLevelVar returns true if the node is an identifier that refers to a
// package-level variable (not in any local scope and not an imported name).
func (t *Transformer) isPkgLevelVar(node *sitter.Node) bool {
	if node == nil || node.Kind() != "identifier" {
		return false
	}
	name := node.Utf8Text(t.source)
	if t.isLocalName(name) {
		return false
	}
	if _, ok := t.importedNames[name]; ok {
		return false
	}
	return true
}

// nodeReturnsJSValue checks whether a tree-sitter node would produce a
// *jsvalue.JSValue expression (e.g. an untyped local, a subscript on one,
// or a method call on one).
func (t *Transformer) nodeReturnsJSValue(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier":
		name := node.Utf8Text(t.source)
		return t.isUntypedLocal(name) || t.jsvalueLocals[name]
	case "subscript_expression":
		objNode := node.ChildByFieldName("object")
		if objNode == nil {
			return false
		}
		// Direct JSValue subscript (untyped local)
		if t.nodeReturnsJSValue(objNode) {
			return true
		}
		// Typed slice with JSValue elements (e.g. []*jsvalue.JSValue)
		if objNode.Kind() == "identifier" && t.jsvalueSliceLocals[objNode.Utf8Text(t.source)] {
			return true
		}
		// Map local subscript returns *jsvalue.JSValue
		if objNode.Kind() == "identifier" && t.mapLocals[objNode.Utf8Text(t.source)] {
			return true
		}
		return false
	case "call_expression":
		fnNode := node.ChildByFieldName("function")
		// Plain function call to untyped local → returns *jsvalue.JSValue
		if fnNode != nil && fnNode.Kind() == "identifier" && t.isUntypedLocal(fnNode.Utf8Text(t.source)) {
			return true
		}
		// Call to jsvalue package functions (NewString, NewNumber, etc.) → returns *jsvalue.JSValue
		if fnNode != nil && fnNode.Kind() == "member_expression" {
			objNode := fnNode.ChildByFieldName("object")
			propNode := fnNode.ChildByFieldName("property")
			if objNode != nil && propNode != nil {
				objText := objNode.Utf8Text(t.source)
				propText := propNode.Utf8Text(t.source)
				// Check if calling jsvalue package functions
				if objText == "jsvalue" || (objNode.Kind() == "identifier" && t.importedNames[objText].goImportPath == "github.com/nnstd/gun/runtime/jsvalue") {
					// Check if this is a JSValue package function that returns *jsvalue.JSValue
					if t.builtins.IsJSValuePackageFunction(propText) {
						return true
					}
				}
				// Object.keys(), Object.entries(), Object.values() return JSValue
				if objText == "Object" && (propText == "keys" || propText == "entries" || propText == "values") {
					return true
				}
				// Check if this is a call to an imported package function
				// By default, imported functions return *jsvalue.JSValue
				if objNode.Kind() == "identifier" {
					if _, isImported := t.importedNames[objText]; isImported {
						// This is a call to an imported function, assume it returns JSValue
						return true
					}
				}
				// [].concat(x) is optimized to just x, so if x returns JSValue, the concat call returns JSValue
				if propText == "concat" && objNode.Kind() == "array" {
					// Check if it's an empty array
					if objNode.NamedChildCount() == 0 {
						// Check if the first argument returns JSValue
						argsNode := node.ChildByFieldName("arguments")
						if argsNode != nil && argsNode.NamedChildCount() > 0 {
							firstArg := argsNode.NamedChild(0)
							if firstArg != nil && t.nodeReturnsJSValue(firstArg) {
								return true
							}
						}
					}
				}
			}
		}
		if fnNode != nil && fnNode.Kind() == "member_expression" {
			objNode := fnNode.ChildByFieldName("object")
			propNode := fnNode.ChildByFieldName("property")
			if objNode != nil && t.nodeReturnsJSValue(objNode) {
				// String methods on JSValue receivers (untyped parameters or JSValue locals)
				// are wrapped to return JSValue, maintaining type consistency.
				if propNode != nil {
					prop := propNode.Utf8Text(t.source)

					// match and exec always return []string, not JSValue
					if prop == "match" || prop == "exec" {
						return false
					}

					// Check if receiver is a JSValue (untyped parameter or JSValue local)
					isJSValueReceiver := objNode.Kind() == "identifier" &&
						(t.isUntypedLocal(objNode.Utf8Text(t.source)) || t.jsvalueLocals[objNode.Utf8Text(t.source)])

					// String methods on JSValue receivers return JSValue (wrapped)
					if isJSValueReceiver && t.builtins.ReturnsJSValueForUntypedReceiver(prop) {
						return true
					}

					// String methods on typed receivers return native Go types
					if !isJSValueReceiver && t.builtins.IsStringMethod(prop) {
						return false
					}
				}
				return true
			}
		}
	case "parenthesized_expression":
		// Unwrap parentheses and check the inner expression
		if node.NamedChildCount() > 0 {
			return t.nodeReturnsJSValue(node.NamedChild(0))
		}
		return false
	case "binary_expression":
		// Logical OR (||) expressions are transformed to IIFEs that return JSValue
		// when either operand is JSValue or when used in JSValue context
		opNode := node.ChildByFieldName("operator")
		if opNode != nil && opNode.Utf8Text(t.source) == "||" {
			leftNode := node.ChildByFieldName("left")
			rightNode := node.ChildByFieldName("right")
			// If either operand returns JSValue, the || expression returns JSValue
			if (leftNode != nil && t.nodeReturnsJSValue(leftNode)) ||
				(rightNode != nil && t.nodeReturnsJSValue(rightNode)) {
				return true
			}
		}
		return false
	case "member_expression":
		objNode := node.ChildByFieldName("object")
		propNode := node.ChildByFieldName("property")
		if objNode != nil && t.nodeReturnsJSValue(objNode) {
			// .length is transformed to .Len() which returns int, not JSValue.
			if propNode != nil && propNode.Utf8Text(t.source) == "length" {
				return false
			}
			return true
		}
		// Map local member access returns *jsvalue.JSValue
		if objNode != nil && objNode.Kind() == "identifier" && t.mapLocals[objNode.Utf8Text(t.source)] {
			if propNode != nil && propNode.Utf8Text(t.source) == "length" {
				return false
			}
			return true
		}
		return false
	}
	return false
}

// isTypedLocal returns true if the name is a local variable with an explicit
// type or a non-JSValue initializer (e.g. bool, string, int).
func (t *Transformer) isTypedLocal(name string) bool {
	for i := len(t.localScopes) - 1; i >= 0; i-- {
		if typed, ok := t.localScopes[i][name]; ok {
			return typed
		}
	}
	return false
}

// isUntypedLocal returns true if the name is a local variable/parameter without
// an explicit type annotation (i.e. it defaults to *jsvalue.JSValue).
func (t *Transformer) isUntypedLocal(name string) bool {
	for i := len(t.localScopes) - 1; i >= 0; i-- {
		if typed, ok := t.localScopes[i][name]; ok {
			return !typed
		}
	}
	return false
}

// jsValueType returns *jsvalue.JSValue and registers the import.
func (t *Transformer) jsValueType() ast.Expr {
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	return jsValuePtrType()
}

func (t *Transformer) transformTopLevel(node *sitter.Node) {
	switch node.Kind() {
	case "function_declaration":
		if d := t.transformFuncDecl(node, false); d != nil {
			t.decls = append(t.decls, d)
		}
	case "lexical_declaration", "variable_declaration":
		decls := t.transformVarDecl(node)
		for _, d := range decls {
			t.decls = append(t.decls, d)
		}
	case "class_declaration":
		classDecls := t.transformClassDecl(node)
		t.decls = append(t.decls, classDecls...)
	case "interface_declaration":
		if d := t.transformInterfaceDecl(node); d != nil {
			t.decls = append(t.decls, d)
		}
	case "enum_declaration":
		decls := t.transformEnumDecl(node)
		t.decls = append(t.decls, decls...)
	case "type_alias_declaration":
		if d := t.transformTypeAlias(node); d != nil {
			t.decls = append(t.decls, d)
		}
	case "export_statement":
		t.transformExport(node)
	case "import_statement":
		t.transformImport(node)
	case "expression_statement":
		// Check for module.exports = expr (CJS default export)
		if node.NamedChildCount() > 0 {
			child := node.NamedChild(0)
			if child.Kind() == "assignment_expression" {
				leftNode := child.ChildByFieldName("left")
				rightNode := child.ChildByFieldName("right")
				if leftNode != nil && rightNode != nil &&
					leftNode.Kind() == "member_expression" &&
					leftNode.Utf8Text(t.source) == "module.exports" {
					// Treat as export default
					switch rightNode.Kind() {
					case "function_expression", "function":
						if d := t.transformAnonFuncAsDefault(rightNode); d != nil {
							t.decls = append(t.decls, d)
						}
					default:
						if expr := t.transformExpr(rightNode); expr != nil {
							t.decls = append(t.decls, varDecl("Default", nil, expr))
						}
					}
					return
				}
			}
		}
		// Top-level expression statements become init() body or are skipped
		if stmt := t.transformStmt(node); stmt != nil {
			initFn := t.getOrCreateMain()
			initFn.Body.List = append(initFn.Body.List, stmt)
		}
	case "comment", "line_comment", "block_comment":
		// skip comments for now
	default:
		// Unknown top-level node, skip
	}
}

func (t *Transformer) transformExport(node *sitter.Node) {
	// Check for "export default ..."
	hasDefault := false
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "default" {
			hasDefault = true
			break
		}
	}

	if hasDefault {
		t.transformExportDefault(node)
		return
	}

	// Detect wildcard re-export: export * from "mod" — skip silently (no Go equivalent)
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "*" {
			if node.ChildByFieldName("source") != nil {
				return
			}
		}
	}

	// export can wrap a declaration — find the inner declaration and capitalize its name
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "export_clause":
			t.transformExportClause(node, child)
		case "function_declaration":
			if d := t.transformFuncDecl(child, true); d != nil {
				t.decls = append(t.decls, d)
			}
		case "lexical_declaration", "variable_declaration":
			decls := t.transformVarDecl(child)
			for _, d := range decls {
				// Capitalize names for export
				switch gd := d.(type) {
				case *ast.GenDecl:
					for _, spec := range gd.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, n := range vs.Names {
								n.Name = capitalize(n.Name)
							}
						}
					}
				}
				t.decls = append(t.decls, d)
			}
		case "class_declaration":
			classDecls := t.transformClassDecl(child)
			t.decls = append(t.decls, classDecls...)
		case "interface_declaration":
			if d := t.transformInterfaceDecl(child); d != nil {
				t.decls = append(t.decls, d)
			}
		case "enum_declaration":
			decls := t.transformEnumDecl(child)
			t.decls = append(t.decls, decls...)
		case "type_alias_declaration":
			if d := t.transformTypeAlias(child); d != nil {
				t.decls = append(t.decls, d)
			}
		}
	}
}

// transformExportDefault handles `export default ...` statements.
// Dispatches by the kind of the exported value.
func (t *Transformer) transformExportDefault(node *sitter.Node) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "function_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				if d := t.transformFuncDecl(child, true); d != nil {
					t.decls = append(t.decls, d)
					// Also create a Default alias so importers can find it
					goName := capitalize(nameNode.Utf8Text(t.source))
					t.decls = append(t.decls, varDecl("Default", nil, ident(goName)))
				}
			} else {
				if d := t.transformAnonFuncAsDefault(child); d != nil {
					t.decls = append(t.decls, d)
				}
			}
			return
		case "function", "function_expression":
			if d := t.transformAnonFuncAsDefault(child); d != nil {
				t.decls = append(t.decls, d)
			}
			return
		case "class_declaration":
			classDecls := t.transformClassDecl(child)
			t.decls = append(t.decls, classDecls...)
			return
		case "object":
			if t.transformExportDefaultObject(child) {
				return
			}
			t.decls = append(t.decls, varDecl("Default", nil, t.transformExpr(child)))
			return
		case "identifier":
			name := child.Utf8Text(t.source)
			t.decls = append(t.decls, varDecl("Default", nil, ident(name)))
			return
		default:
			if expr := t.transformExpr(child); expr != nil {
				t.decls = append(t.decls, varDecl("Default", nil, expr))
			}
			return
		}
	}
}

// transformExportDefaultObject tries to match the Hono server pattern
// `export default { port: X, fetch: Y }`. Returns true if matched.
func (t *Transformer) transformExportDefaultObject(node *sitter.Node) bool {
	var portExpr ast.Expr
	var fetchExpr ast.Expr

	for j := uint(0); j < node.NamedChildCount(); j++ {
		pair := node.NamedChild(j)
		if pair.Kind() != "pair" {
			continue
		}
		keyNode := pair.ChildByFieldName("key")
		valNode := pair.ChildByFieldName("value")
		if keyNode == nil || valNode == nil {
			continue
		}
		key := keyNode.Utf8Text(t.source)
		switch key {
		case "port":
			portExpr = t.transformExpr(valNode)
		case "fetch":
			fetchExpr = t.transformExpr(valNode)
		}
	}

	if portExpr == nil || fetchExpr == nil {
		return false
	}

	t.addImport("fmt")
	t.addImport("log")
	t.addImport("net/http")

	mainFn := t.getOrCreateMain()

	portStr := "3000"
	if lit, ok := portExpr.(*ast.BasicLit); ok {
		portStr = lit.Value
	}
	printStmt := exprStmt(callExpr(
		selectorExpr(ident("fmt"), "Println"),
		stringLit("Listening on :"+portStr),
	))

	listenStmt := exprStmt(callExpr(
		selectorExpr(ident("log"), "Fatal"),
		callExpr(
			selectorExpr(ident("http"), "ListenAndServe"),
			stringLit(":"+portStr),
			fetchExpr,
		),
	))

	mainFn.Body.List = append(mainFn.Body.List, printStmt, listenStmt)
	return true
}

// transformAnonFuncAsDefault handles `export default function() { ... }` (no name).
func (t *Transformer) transformAnonFuncAsDefault(node *sitter.Node) *ast.FuncDecl {
	paramsNode := node.ChildByFieldName("parameters")
	returnTypeNode := node.ChildByFieldName("return_type")
	bodyNode := node.ChildByFieldName("body")

	params, paramStmts := t.transformParams(paramsNode)
	var results *ast.FieldList
	if returnTypeNode != nil {
		retType := t.getTypeAnnotation(returnTypeNode)
		if retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	if len(paramStmts) > 0 {
		body.List = append(paramStmts, body.List...)
	}

	if results == nil {
		if inferred := inferReturnType(body); inferred != nil {
			results = inferred
		} else if hasReturnValue(body) {
			results = fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			t.addImport("fmt")
			wrapReturnsWithJSValue(body)
		}
	}

	return funcDecl("Default", params, results, body)
}

// transformExportClause handles `export { foo, bar }`, `export { foo as bar }`,
// and `export { x } from "mod"`.
func (t *Transformer) transformExportClause(exportNode *sitter.Node, clause *sitter.Node) {
	sourceNode := exportNode.ChildByFieldName("source")
	var reexportMod string
	if sourceNode != nil {
		reexportMod = strings.Trim(sourceNode.Utf8Text(t.source), "'\"")
	}

	for i := uint(0); i < clause.NamedChildCount(); i++ {
		spec := clause.NamedChild(i)
		if spec.Kind() != "export_specifier" {
			continue
		}

		nameNode := spec.ChildByFieldName("name")
		aliasNode := spec.ChildByFieldName("alias")
		if nameNode == nil {
			continue
		}

		origName := nameNode.Utf8Text(t.source)
		exportedName := origName
		if aliasNode != nil {
			exportedName = aliasNode.Utf8Text(t.source)
		}

		// Skip string-aliased exports like `export {X as 'module.exports'}` — CJS interop, no Go equivalent
		if strings.HasPrefix(exportedName, "'") || strings.HasPrefix(exportedName, "\"") {
			continue
		}

		goName := capitalize(exportedName)

		if reexportMod != "" {
			t.transformReexport(origName, goName, reexportMod)
		} else {
			t.decls = append(t.decls, varDecl(goName, nil, ident(origName)))
		}
	}
}

// transformReexport handles a single re-exported symbol from a module.
func (t *Transformer) transformReexport(origName, goName, modulePath string) {
	// Check knownSymbols for a specific mapping
	if symTable, ok := knownSymbols[modulePath]; ok {
		if sym, ok := symTable[origName]; ok {
			if sym.goPkgName != "" && sym.goPkgName != filepath.Base(sym.goImportPath) {
				t.addAliasedImport(sym.goImportPath, sym.goPkgName)
			} else {
				t.addImport(sym.goImportPath)
			}
			t.decls = append(t.decls, varDecl(goName, nil, selectorExpr(ident(sym.goPkgName), sym.goSymbol)))
			return
		}
	}

	// Fall back to module mapping with capitalized name
	mod, isKnown := knownModules[modulePath]
	if !isKnown {
		mod = t.resolveModulePath(modulePath)
	}

	// Same-package: no import needed, reference symbol directly
	if mod.goPath == "" {
		t.decls = append(t.decls, varDecl(goName, nil, ident(capitalize(origName))))
		return
	}

	if mod.goName != "" && mod.goName != filepath.Base(mod.goPath) {
		t.addAliasedImport(mod.goPath, mod.goName)
	} else {
		t.addImport(mod.goPath)
	}
	t.decls = append(t.decls, varDecl(goName, nil, selectorExpr(ident(mod.goName), capitalize(origName))))
}

// getOrCreateMain returns the main() (or init()) function, creating it if needed.
// Uses main() when package is "main" so the output is directly runnable.
func (t *Transformer) getOrCreateMain() *ast.FuncDecl {
	name := "init"
	if t.pkgName == "main" {
		name = "main"
	}
	for _, d := range t.decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fd
		}
	}
	fn := funcDecl(name, fieldList(), nil, blockStmt())
	t.decls = append(t.decls, fn)
	return fn
}

// nodeText returns the source text of a node.
func (t *Transformer) nodeText(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	return node.Utf8Text(t.source)
}
