package compiler

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Transformer walks a tree-sitter TypeScript AST and builds a go/ast.File.
type Transformer struct {
	source        []byte
	pkgName       string
	moduleName    string                     // Go module name from go.mod (for relative imports)
	decls         []ast.Decl
	imports       map[string]string          // Go import path → alias (empty = no alias)
	importedNames map[string]resolvedImport  // TS name → Go resolution
	varTypes      map[string]string          // variable name → module type (e.g. "app" → "hono")
}

func newTransformer(source []byte, pkgName, moduleName string) *Transformer {
	return &Transformer{
		source:        source,
		pkgName:       pkgName,
		moduleName:    moduleName,
		imports:       make(map[string]string),
		importedNames: make(map[string]resolvedImport),
		varTypes:      make(map[string]string),
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
		// Top-level expression statements become init() body or are skipped
		// For now, wrap in an init function if it's a call
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

	params := t.transformParams(paramsNode)
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
