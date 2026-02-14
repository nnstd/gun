package compiler

import (
	"go/ast"
	"go/token"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Transformer walks a tree-sitter TypeScript AST and builds a go/ast.File.
type Transformer struct {
	source        []byte
	pkgName       string
	moduleName    string // Go module name from go.mod (for relative imports)
	decls         []ast.Decl
	imports       map[string]string          // Go import path → alias (empty = no alias)
	importedNames map[string]resolvedImport // TS name → Go resolution
}

func newTransformer(source []byte, pkgName, moduleName string) *Transformer {
	return &Transformer{
		source:        source,
		pkgName:       pkgName,
		moduleName:    moduleName,
		imports:       make(map[string]string),
		importedNames: make(map[string]resolvedImport),
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
	// export can wrap a declaration — find the inner declaration and capitalize its name
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
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
