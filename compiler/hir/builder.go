package hir

import (
	"strings"

	"github.com/nnstd/gun/compiler/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Builder walks a tree-sitter CST and produces an HIR Module.
type Builder struct {
	source  []byte
	symtab  *symbol.Table
	pkgName string
	path    string
}

type classParts struct {
	parent      Expr
	constructor *ClassConstructor
	methods     []*ClassMethod
	properties  []*ClassProperty
	staticInits []Expr
}

// BuildModule creates an HIR Module from a tree-sitter CST root node.
func BuildModule(root *sitter.Node, source []byte, pkgName string) *Module {
	return BuildModuleWithPath(root, source, pkgName, "")
}

// BuildModuleWithPath creates an HIR Module with original source path metadata.
func BuildModuleWithPath(root *sitter.Node, source []byte, pkgName, sourcePath string) *Module {
	symtab := symbol.NewTable()
	b := &Builder{
		source:  source,
		symtab:  symtab,
		pkgName: pkgName,
		path:    sourcePath,
	}
	mod := &Module{
		Package:     pkgName,
		SymbolTable: symtab,
		SourcePath:  sourcePath,
		SourceSize:  len(source),
	}

	// Pre-pass: register top-level declarations for JS/module hoisting.
	b.hoistTopLevelDecls(root)

	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		b.buildTopLevel(mod, child)
	}

	return mod
}

func (b *Builder) span(node *sitter.Node) *SourceSpan {
	if node == nil {
		return nil
	}
	start := node.StartPosition()
	end := node.EndPosition()
	return &SourceSpan{
		StartByte:   int(node.StartByte()),
		EndByte:     int(node.EndByte()),
		StartLine:   int(start.Row) + 1,
		StartColumn: int(start.Column) + 1,
		EndLine:     int(end.Row) + 1,
		EndColumn:   int(end.Column) + 1,
	}
}

// nodeText returns the UTF-8 text of a CST node.
func (b *Builder) nodeText(node *sitter.Node) string {
	return node.Utf8Text(b.source)
}

// --------------------------------------------------------------------
// Top-level dispatch
// --------------------------------------------------------------------

func (b *Builder) buildTopLevel(mod *Module, node *sitter.Node) {
	switch node.Kind() {
	case "function_declaration":
		if d := b.buildFuncDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "lexical_declaration", "variable_declaration":
		if d := b.buildVarDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "class_declaration":
		if d := b.buildClassDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "interface_declaration":
		if d := b.buildInterfaceDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "enum_declaration":
		if d := b.buildEnumDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "type_alias_declaration":
		if d := b.buildTypeAliasDecl(node, false); d != nil {
			mod.Declarations = append(mod.Declarations, d)
		}
	case "export_statement":
		b.buildExport(mod, node)
	case "import_statement":
		if d := b.buildImportDecl(node); d != nil {
			mod.Imports = append(mod.Imports, d)
		}
	case "expression_statement":
		// Detect module.exports = expr → treat as export default
		if b.isModuleExportsAssign(node) {
			expr := b.extractModuleExportsValue(node)
			if expr != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{
					Decl: &VarDecl{
						Kind:     VarConst,
						Exported: true,
						Declarators: []*Declarator{{
							Symbol: b.symtab.Define("default", symbol.KindVariable),
							Init:   expr,
						}},
					},
					IsDefault: true,
				})
				return
			}
		}
		if s := b.buildStmt(node); s != nil {
			mod.Declarations = append(mod.Declarations, &TopLevelStmt{Stmt: s, Span: b.span(node)})
		}
	case "comment", "line_comment", "block_comment":
		// skip
	}
}

// --------------------------------------------------------------------
// Declarations
// --------------------------------------------------------------------

func (b *Builder) buildFuncDecl(node *sitter.Node, exported bool) *FuncDecl {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := b.nodeText(nameNode)
	// Reuse pre-hoisted symbol if it exists (from hoistTopLevelFunctions)
	sym := b.symtab.Lookup(name)
	if sym == nil || sym.Kind != symbol.KindFunction {
		sym = b.symtab.Define(name, symbol.KindFunction)
	}
	sym.Exported = exported

	// Push a scope for function parameters so they don't leak into the
	// enclosing scope (e.g. sibling function declarations must not see
	// each other's parameters).
	b.symtab.PushScope()

	paramsNode := node.ChildByFieldName("parameters")
	params := b.buildParams(paramsNode)

	// Check for async
	isAsync := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "async" {
			isAsync = true
			break
		}
	}

	bodyNode := node.ChildByFieldName("body")
	var body *BlockStmt
	if bodyNode != nil {
		body = b.buildBlock(bodyNode)
	}

	b.symtab.PopScope()

	// Record param count in symbol
	sym.FuncInfo = &symbol.FuncInfo{ParamCount: len(params)}

	return &FuncDecl{
		Symbol:   sym,
		Params:   params,
		Body:     body,
		Exported: exported,
		IsAsync:  isAsync,
		Span:     b.span(node),
	}
}

func (b *Builder) buildVarDecl(node *sitter.Node, exported bool) *VarDecl {
	kind := VarLet
	if node.Kind() == "variable_declaration" {
		kind = VarVar
	} else {
		// lexical_declaration: check for const vs let
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child.Kind() == "const" {
				kind = VarConst
				break
			}
		}
	}

	var declarators []*Declarator
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "variable_declarator" {
			continue
		}
		d := b.buildDeclarator(child, exported)
		if d != nil {
			declarators = append(declarators, d)
		}
	}

	if len(declarators) == 0 {
		return nil
	}
	return &VarDecl{
		Declarators: declarators,
		Kind:        kind,
		Exported:    exported,
	}
}

func (b *Builder) buildDeclarator(node *sitter.Node, exported bool) *Declarator {
	nameNode := node.ChildByFieldName("name")
	valueNode := node.ChildByFieldName("value")

	if nameNode == nil {
		var init Expr
		if valueNode != nil {
			init = b.buildExpr(valueNode)
		}
		return &Declarator{Init: init}
	}

	switch nameNode.Kind() {
	case "identifier":
		name := b.nodeText(nameNode)
		var sym *symbol.Symbol
		if b.symtab.IsGlobalScope() {
			sym = b.symtab.LookupLocal(name)
		}
		if sym == nil {
			sym = b.symtab.Define(name, symbol.KindVariable)
		}
		sym.Exported = exported
		var init Expr
		if valueNode != nil {
			init = b.buildExpr(valueNode)
		}
		return &Declarator{Symbol: sym, Init: init}
	case "object_pattern":
		var init Expr
		if valueNode != nil {
			init = b.buildExpr(valueNode)
		}
		pat := b.buildObjectPattern(nameNode)
		return &Declarator{Pattern: pat, Init: init}
	case "array_pattern":
		var init Expr
		if valueNode != nil {
			init = b.buildExpr(valueNode)
		}
		pat := b.buildArrayPattern(nameNode)
		return &Declarator{Pattern: pat, Init: init}
	default:
		// Fallback: treat as identifier
		name := b.nodeText(nameNode)
		var sym *symbol.Symbol
		if b.symtab.IsGlobalScope() {
			sym = b.symtab.LookupLocal(name)
		}
		if sym == nil {
			sym = b.symtab.Define(name, symbol.KindVariable)
		}
		sym.Exported = exported
		var init Expr
		if valueNode != nil {
			init = b.buildExpr(valueNode)
		}
		return &Declarator{Symbol: sym, Init: init}
	}
}

func (b *Builder) buildClassDecl(node *sitter.Node, exported bool) *ClassDecl {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := b.nodeText(nameNode)
	var sym *symbol.Symbol
	if b.symtab.IsGlobalScope() {
		sym = b.symtab.LookupLocal(name)
	}
	if sym == nil {
		sym = b.symtab.Define(name, symbol.KindClass)
	}
	sym.Exported = exported
	parts := b.buildClassParts(node)

	decl := &ClassDecl{
		Symbol:      sym,
		Parent:      parts.parent,
		Constructor: parts.constructor,
		Methods:     parts.methods,
		Properties:  parts.properties,
		StaticInits: parts.staticInits,
		Exported:    exported,
		Span:        b.span(node),
	}
	return decl
}

func (b *Builder) buildClassParts(node *sitter.Node) *classParts {
	parts := &classParts{}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "class_heritage" {
			for j := uint(0); j < child.NamedChildCount(); j++ {
				hChild := child.NamedChild(j)
				if hChild.Kind() == "extends_clause" && hChild.NamedChildCount() > 0 {
					parts.parent = b.buildExpr(hChild.NamedChild(0))
				}
			}
		}
		if child.Kind() == "class_body" {
			b.buildClassBody(parts, child)
			break
		}
	}

	return parts
}

func (b *Builder) buildClassBody(parts *classParts, node *sitter.Node) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "method_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := b.nodeText(nameNode)

			// Computed property name: [(expr1, expr2, ..., name)] or [varName]
			// Extract side-effect expressions and use the last as the method name.
			var computedExpr Expr
			isPrivate := nameNode.Kind() == "private_property_identifier"
			if isPrivate {
				name = strings.TrimPrefix(name, "#")
			}
			if nameNode.Kind() == "computed_property_name" {
				sideEffects, methodName := b.extractComputedPropertyName(nameNode)
				parts.staticInits = append(parts.staticInits, sideEffects...)
				name = methodName
				// For simple computed names like [kSymbol], build expression for dynamic dispatch
				if nameNode.NamedChildCount() == 1 {
					inner := nameNode.NamedChild(0)
					if inner.Kind() == "identifier" || inner.Kind() == "member_expression" {
						computedExpr = b.buildExpr(inner)
					}
				}
			}

			paramsNode := child.ChildByFieldName("parameters")
			params := b.buildParams(paramsNode)

			bodyNode := child.ChildByFieldName("body")
			var body *BlockStmt
			if bodyNode != nil {
				body = b.buildBlock(bodyNode)
			}

			isStatic := false
			isGetter := false
			isSetter := false
			isAsync := false
			for j := uint(0); j < child.ChildCount(); j++ {
				ck := child.Child(j).Kind()
				if ck == "static" {
					isStatic = true
				} else if ck == "get" {
					isGetter = true
				} else if ck == "set" {
					isSetter = true
				} else if ck == "async" {
					isAsync = true
				}
			}

			if name == "constructor" {
				parts.constructor = &ClassConstructor{
					Params: params,
					Body:   body,
					Span:   b.span(child),
				}
			} else {
				parts.methods = append(parts.methods, &ClassMethod{
					Name:      name,
					Params:    params,
					Body:      body,
					IsAsync:   isAsync,
					IsStatic:  isStatic,
					IsGetter:  isGetter,
					IsSetter:  isSetter,
					IsPrivate: isPrivate,
					Computed:  computedExpr,
					Span:      b.span(child),
				})
			}
		case "public_field_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := b.nodeText(nameNode)
			isPrivate := nameNode.Kind() == "private_property_identifier"
			if isPrivate {
				name = strings.TrimPrefix(name, "#")
			}
			var value Expr
			valueNode := child.ChildByFieldName("value")
			if valueNode != nil {
				value = b.buildExpr(valueNode)
			}
			isStatic := false
			for j := uint(0); j < child.ChildCount(); j++ {
				if child.Child(j).Kind() == "static" {
					isStatic = true
				}
			}
			parts.properties = append(parts.properties, &ClassProperty{
				Name:      name,
				Value:     value,
				IsStatic:  isStatic,
				IsPrivate: isPrivate,
			})
		}
	}
}

func (b *Builder) buildEnumDecl(node *sitter.Node, exported bool) *EnumDecl {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := b.nodeText(nameNode)
	var sym *symbol.Symbol
	if b.symtab.IsGlobalScope() {
		sym = b.symtab.LookupLocal(name)
	}
	if sym == nil {
		sym = b.symtab.Define(name, symbol.KindEnum)
	}
	sym.Exported = exported

	var bodyNode *sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "enum_body" {
			bodyNode = child
			break
		}
	}

	var members []*EnumMember
	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			child := bodyNode.NamedChild(i)
			if child.Kind() != "enum_assignment" && child.Kind() != "property_identifier" {
				// Also handle identifier
				if child.Kind() == "identifier" {
					members = append(members, &EnumMember{Name: b.nodeText(child)})
				}
				continue
			}
			if child.Kind() == "property_identifier" {
				members = append(members, &EnumMember{Name: b.nodeText(child)})
				continue
			}
			// enum_assignment: name = value
			mNameNode := child.ChildByFieldName("name")
			mValueNode := child.ChildByFieldName("value")
			if mNameNode == nil {
				continue
			}
			em := &EnumMember{Name: b.nodeText(mNameNode)}
			if mValueNode != nil {
				em.Value = b.buildExpr(mValueNode)
			}
			members = append(members, em)
		}
	}

	return &EnumDecl{
		Symbol:   sym,
		Members:  members,
		Exported: exported,
	}
}

func (b *Builder) buildInterfaceDecl(node *sitter.Node, exported bool) *InterfaceDecl {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := b.nodeText(nameNode)
	var sym *symbol.Symbol
	if b.symtab.IsGlobalScope() {
		sym = b.symtab.LookupLocal(name)
	}
	if sym == nil {
		sym = b.symtab.Define(name, symbol.KindType)
	}
	sym.Exported = exported

	var bodyNode *sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "object_type" || child.Kind() == "interface_body" {
			bodyNode = child
			break
		}
	}

	var members []*InterfaceMember
	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			child := bodyNode.NamedChild(i)
			switch child.Kind() {
			case "property_signature":
				memberName := ""
				memberType := ""
				if n := child.ChildByFieldName("name"); n != nil {
					memberName = b.nodeText(n)
				}
				if t := child.ChildByFieldName("type"); t != nil {
					memberType = b.nodeText(t)
				}
				members = append(members, &InterfaceMember{
					Name: memberName,
					Type: memberType,
				})
			case "method_signature":
				memberName := ""
				paramCount := 0
				if n := child.ChildByFieldName("name"); n != nil {
					memberName = b.nodeText(n)
				}
				if p := child.ChildByFieldName("parameters"); p != nil {
					paramCount = int(p.NamedChildCount())
				}
				members = append(members, &InterfaceMember{
					Name:       memberName,
					IsMethod:   true,
					ParamCount: paramCount,
				})
			}
		}
	}

	return &InterfaceDecl{
		Symbol:   sym,
		Members:  members,
		Exported: exported,
	}
}

func (b *Builder) buildTypeAliasDecl(node *sitter.Node, exported bool) *TypeAliasDecl {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := b.nodeText(nameNode)
	var sym *symbol.Symbol
	if b.symtab.IsGlobalScope() {
		sym = b.symtab.LookupLocal(name)
	}
	if sym == nil {
		sym = b.symtab.Define(name, symbol.KindType)
	}
	sym.Exported = exported

	typeStr := ""
	if t := node.ChildByFieldName("value"); t != nil {
		typeStr = b.nodeText(t)
	}

	return &TypeAliasDecl{
		Symbol:   sym,
		Type:     typeStr,
		Exported: exported,
	}
}

func (b *Builder) buildImportDecl(node *sitter.Node) *ImportDecl {
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return nil
	}
	modulePath := b.nodeText(sourceNode)
	modulePath = strings.Trim(modulePath, "'\"")
	modulePath = strings.TrimPrefix(modulePath, "node:")

	typeOnly := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "type" {
			typeOnly = true
			break
		}
	}

	decl := &ImportDecl{
		ModulePath: modulePath,
		TypeOnly:   typeOnly,
	}

	// Find import_clause
	var clauseNode *sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "import_clause" {
			clauseNode = child
			break
		}
	}
	if clauseNode == nil {
		return decl
	}

	for i := uint(0); i < clauseNode.NamedChildCount(); i++ {
		child := clauseNode.NamedChild(i)
		switch child.Kind() {
		case "identifier":
			// Default import
			localName := b.nodeText(child)
			sym := b.symtab.LookupLocal(localName)
			if sym == nil {
				sym = b.symtab.Define(localName, symbol.KindImport)
			}
			decl.Default = &ImportBinding{
				LocalName:    localName,
				OriginalName: "default",
				Symbol:       sym,
			}
		case "named_imports":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				spec := child.NamedChild(j)
				if spec.Kind() != "import_specifier" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				aliasNode := spec.ChildByFieldName("alias")
				if nameNode == nil {
					continue
				}
				origName := b.nodeText(nameNode)
				localName := origName
				if aliasNode != nil {
					localName = b.nodeText(aliasNode)
				}
				sym := b.symtab.LookupLocal(localName)
				if sym == nil {
					sym = b.symtab.Define(localName, symbol.KindImport)
				}
				decl.Named = append(decl.Named, &ImportBinding{
					LocalName:    localName,
					OriginalName: origName,
					Symbol:       sym,
				})
			}
		case "namespace_import":
			var alias string
			for j := uint(0); j < child.NamedChildCount(); j++ {
				if child.NamedChild(j).Kind() == "identifier" {
					alias = b.nodeText(child.NamedChild(j))
				}
			}
			if alias != "" {
				sym := b.symtab.LookupLocal(alias)
				if sym == nil {
					sym = b.symtab.Define(alias, symbol.KindImport)
				}
				decl.Namespace = &ImportBinding{
					LocalName:    alias,
					OriginalName: "*",
					Symbol:       sym,
				}
			}
		}
	}

	return decl
}

func (b *Builder) buildExport(mod *Module, node *sitter.Node) {
	// Check for default
	hasDefault := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "default" {
			hasDefault = true
			break
		}
	}

	// Wildcard re-export: export * from "mod"
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "*" {
			if node.ChildByFieldName("source") != nil {
				hasNamespaceAlias := false
				for j := uint(0); j < node.NamedChildCount(); j++ {
					if node.NamedChild(j).Kind() == "namespace_export" {
						hasNamespaceAlias = true
						break
					}
				}
				if hasNamespaceAlias {
					continue
				}
				sourceNode := node.ChildByFieldName("source")
				mod.Declarations = append(mod.Declarations, &ExportDecl{
					FromModule: strings.Trim(b.nodeText(sourceNode), "'\""),
					IsWildcard: true,
				})
				return
			}
		}
	}

	if hasDefault {
		b.buildExportDefault(mod, node)
		return
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "namespace_export":
			sourceNode := node.ChildByFieldName("source")
			if sourceNode == nil || child.NamedChildCount() == 0 {
				continue
			}
			aliasNode := child.NamedChild(0)
			ed := &ExportDecl{
				FromModule: strings.Trim(b.nodeText(sourceNode), "'\""),
				Names: []ExportName{{
					LocalName:    "*",
					ExportedName: b.nodeText(aliasNode),
				}},
			}
			mod.Declarations = append(mod.Declarations, ed)
		case "export_clause":
			ed := &ExportDecl{}
			if sourceNode := node.ChildByFieldName("source"); sourceNode != nil {
				ed.FromModule = strings.Trim(b.nodeText(sourceNode), "'\"")
			}
			for j := uint(0); j < child.NamedChildCount(); j++ {
				spec := child.NamedChild(j)
				if spec.Kind() != "export_specifier" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				aliasNode := spec.ChildByFieldName("alias")
				if nameNode == nil {
					continue
				}
				localName := b.nodeText(nameNode)
				exportedName := localName
				if aliasNode != nil {
					exportedName = b.nodeText(aliasNode)
					if (strings.HasPrefix(exportedName, "'") && strings.HasSuffix(exportedName, "'")) ||
						(strings.HasPrefix(exportedName, "\"") && strings.HasSuffix(exportedName, "\"")) {
						// CJS interop aliases like `export { X as 'module.exports' }`
						// do not have a Go-level export equivalent.
						continue
					}
				}
				ed.Names = append(ed.Names, ExportName{
					LocalName:    localName,
					ExportedName: exportedName,
				})
			}
			mod.Declarations = append(mod.Declarations, ed)
		case "function_declaration":
			if d := b.buildFuncDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		case "lexical_declaration", "variable_declaration":
			if d := b.buildVarDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		case "class_declaration":
			if d := b.buildClassDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		case "interface_declaration":
			if d := b.buildInterfaceDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		case "enum_declaration":
			if d := b.buildEnumDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		case "type_alias_declaration":
			if d := b.buildTypeAliasDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{Decl: d})
			}
		}
	}
}

func (b *Builder) buildExportDefault(mod *Module, node *sitter.Node) {
	// Find the first named child after "default"
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "function_declaration":
			if d := b.buildFuncDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{
					Decl:      d,
					IsDefault: true,
				})
			}
			return
		case "class_declaration":
			if d := b.buildClassDecl(child, true); d != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{
					Decl:      d,
					IsDefault: true,
				})
			}
			return
		default:
			// Expression: export default expr
			expr := b.buildExpr(child)
			if expr != nil {
				mod.Declarations = append(mod.Declarations, &ExportDecl{
					Decl: &VarDecl{
						Kind:     VarConst,
						Exported: true,
						Declarators: []*Declarator{{
							Symbol: b.symtab.Define("default", symbol.KindVariable),
							Init:   expr,
						}},
					},
					IsDefault: true,
				})
			}
			return
		}
	}
}

// --------------------------------------------------------------------
// Parameters
// --------------------------------------------------------------------

func (b *Builder) buildParams(node *sitter.Node) []*Param {
	if node == nil {
		return nil
	}
	var params []*Param
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "comment" {
			continue
		}
		p := b.buildParam(child)
		if p != nil {
			params = append(params, p)
		}
	}
	return params
}

func (b *Builder) buildParam(node *sitter.Node) *Param {
	switch node.Kind() {
	case "required_parameter", "optional_parameter":
		patNode := node.ChildByFieldName("pattern")
		if patNode == nil {
			// Try name field
			patNode = node.ChildByFieldName("name")
		}
		if patNode == nil && node.NamedChildCount() > 0 {
			patNode = node.NamedChild(0)
		}
		if patNode == nil {
			return nil
		}

		typeAnno := ""
		if t := node.ChildByFieldName("type"); t != nil {
			typeAnno = b.nodeText(t)
		}

		var defaultExpr Expr
		if v := node.ChildByFieldName("value"); v != nil {
			defaultExpr = b.buildExpr(v)
		}

		switch patNode.Kind() {
		case "identifier":
			name := b.nodeText(patNode)
			sym := b.symtab.Define(name, symbol.KindParameter)
			return &Param{Symbol: sym, Default: defaultExpr, TypeAnno: typeAnno}
		case "object_pattern":
			pat := b.buildObjectPattern(patNode)
			return &Param{Pattern: pat, Default: defaultExpr, TypeAnno: typeAnno}
		case "array_pattern":
			pat := b.buildArrayPattern(patNode)
			return &Param{Pattern: pat, Default: defaultExpr, TypeAnno: typeAnno}
		case "rest_pattern":
			if patNode.NamedChildCount() > 0 {
				inner := patNode.NamedChild(0)
				name := b.nodeText(inner)
				sym := b.symtab.Define(name, symbol.KindParameter)
				return &Param{Symbol: sym, Rest: true, TypeAnno: typeAnno}
			}
		case "assignment_pattern":
			leftNode := patNode.ChildByFieldName("left")
			rightNode := patNode.ChildByFieldName("right")
			if leftNode != nil {
				name := b.nodeText(leftNode)
				sym := b.symtab.Define(name, symbol.KindParameter)
				if rightNode != nil {
					defaultExpr = b.buildExpr(rightNode)
				}
				return &Param{Symbol: sym, Default: defaultExpr, TypeAnno: typeAnno}
			}
		default:
			name := b.nodeText(patNode)
			sym := b.symtab.Define(name, symbol.KindParameter)
			return &Param{Symbol: sym, Default: defaultExpr, TypeAnno: typeAnno}
		}
	case "rest_parameter":
		if node.NamedChildCount() > 0 {
			inner := node.NamedChild(0)
			name := b.nodeText(inner)
			sym := b.symtab.Define(name, symbol.KindParameter)
			return &Param{Symbol: sym, Rest: true}
		}
	case "identifier":
		name := b.nodeText(node)
		sym := b.symtab.Define(name, symbol.KindParameter)
		return &Param{Symbol: sym}
	}
	return nil
}

// --------------------------------------------------------------------
// Patterns
// --------------------------------------------------------------------

func (b *Builder) buildObjectPattern(node *sitter.Node) *ObjectPattern {
	pat := &ObjectPattern{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "shorthand_property_identifier_pattern", "shorthand_property_identifier":
			name := b.nodeText(child)
			sym := b.symtab.Define(name, symbol.KindVariable)
			pat.Properties = append(pat.Properties, &ObjectPatternProp{
				Key:   name,
				Value: sym,
			})
		case "pair_pattern":
			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")
			if keyNode != nil && valueNode != nil {
				key := b.nodeText(keyNode)
				prop := &ObjectPatternProp{Key: key}
				switch valueNode.Kind() {
				case "object_pattern":
					prop.Pattern = b.buildObjectPattern(valueNode)
				case "array_pattern":
					prop.Pattern = b.buildArrayPattern(valueNode)
				default:
					valName := b.nodeText(valueNode)
					prop.Value = b.symtab.Define(valName, symbol.KindVariable)
				}
				pat.Properties = append(pat.Properties, prop)
			}
		case "object_assignment_pattern":
			// {key = default}
			leftNode := child.ChildByFieldName("left")
			rightNode := child.ChildByFieldName("right")
			if leftNode != nil {
				name := b.nodeText(leftNode)
				sym := b.symtab.Define(name, symbol.KindVariable)
				var def Expr
				if rightNode != nil {
					def = b.buildExpr(rightNode)
				}
				pat.Properties = append(pat.Properties, &ObjectPatternProp{
					Key:     name,
					Value:   sym,
					Default: def,
				})
			}
		case "rest_pattern":
			if child.NamedChildCount() > 0 {
				name := b.nodeText(child.NamedChild(0))
				sym := b.symtab.Define(name, symbol.KindVariable)
				pat.Rest = sym
			}
		}
	}
	return pat
}

func (b *Builder) buildArrayPattern(node *sitter.Node) *ArrayPattern {
	pat := &ArrayPattern{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "identifier":
			name := b.nodeText(child)
			sym := b.symtab.Define(name, symbol.KindVariable)
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Symbol: sym})
		case "array_pattern":
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Pattern: b.buildArrayPattern(child)})
		case "object_pattern":
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Pattern: b.buildObjectPattern(child)})
		case "assignment_pattern":
			leftNode := child.ChildByFieldName("left")
			rightNode := child.ChildByFieldName("right")
			if leftNode != nil {
				elem := &ArrayPatternElem{}
				switch leftNode.Kind() {
				case "object_pattern":
					elem.Pattern = b.buildObjectPattern(leftNode)
				case "array_pattern":
					elem.Pattern = b.buildArrayPattern(leftNode)
				default:
					name := b.nodeText(leftNode)
					elem.Symbol = b.symtab.Define(name, symbol.KindVariable)
				}
				var def Expr
				if rightNode != nil {
					def = b.buildExpr(rightNode)
				}
				elem.Default = def
				pat.Elements = append(pat.Elements, elem)
			}
		case "rest_pattern":
			if child.NamedChildCount() > 0 {
				name := b.nodeText(child.NamedChild(0))
				sym := b.symtab.Define(name, symbol.KindVariable)
				pat.Rest = sym
			}
		}
	}
	return pat
}

// isModuleExportsAssign checks if an expression_statement is `module.exports = expr`.
func (b *Builder) isModuleExportsAssign(node *sitter.Node) bool {
	if node.NamedChildCount() == 0 {
		return false
	}
	child := node.NamedChild(0)
	if child.Kind() != "assignment_expression" {
		return false
	}
	left := child.ChildByFieldName("left")
	if left == nil || left.Kind() != "member_expression" {
		return false
	}
	return b.nodeText(left) == "module.exports"
}

// extractModuleExportsValue returns the RHS expression of `module.exports = expr`.
func (b *Builder) extractModuleExportsValue(node *sitter.Node) Expr {
	child := node.NamedChild(0)
	right := child.ChildByFieldName("right")
	if right == nil {
		return nil
	}
	return b.buildExpr(right)
}

// lookupOrDefine tries Lookup first (for assignment context where vars exist),
// falls back to Define if not found.
func (b *Builder) lookupOrDefine(name string, kind symbol.Kind) *symbol.Symbol {
	if sym := b.symtab.Lookup(name); sym != nil {
		return sym
	}
	return b.symtab.Define(name, kind)
}

// buildObjectPatternLookup builds an object pattern using Lookup for existing variables.
func (b *Builder) buildObjectPatternLookup(node *sitter.Node) *ObjectPattern {
	pat := &ObjectPattern{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "shorthand_property_identifier_pattern", "shorthand_property_identifier":
			name := b.nodeText(child)
			sym := b.lookupOrDefine(name, symbol.KindVariable)
			pat.Properties = append(pat.Properties, &ObjectPatternProp{Key: name, Value: sym})
		case "pair_pattern":
			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")
			if keyNode != nil && valueNode != nil {
				key := b.nodeText(keyNode)
				prop := &ObjectPatternProp{Key: key}
				switch valueNode.Kind() {
				case "object_pattern":
					prop.Pattern = b.buildObjectPatternLookup(valueNode)
				case "array_pattern":
					prop.Pattern = b.buildArrayPatternLookup(valueNode)
				default:
					valName := b.nodeText(valueNode)
					prop.Value = b.lookupOrDefine(valName, symbol.KindVariable)
				}
				pat.Properties = append(pat.Properties, prop)
			}
		case "rest_pattern":
			if child.NamedChildCount() > 0 {
				name := b.nodeText(child.NamedChild(0))
				sym := b.lookupOrDefine(name, symbol.KindVariable)
				pat.Rest = sym
			}
		}
	}
	return pat
}

// buildArrayPatternLookup builds an array pattern using Lookup for existing variables.
func (b *Builder) buildArrayPatternLookup(node *sitter.Node) *ArrayPattern {
	pat := &ArrayPattern{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "identifier":
			name := b.nodeText(child)
			sym := b.lookupOrDefine(name, symbol.KindVariable)
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Symbol: sym})
		case "array_pattern":
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Pattern: b.buildArrayPatternLookup(child)})
		case "object_pattern":
			pat.Elements = append(pat.Elements, &ArrayPatternElem{Pattern: b.buildObjectPatternLookup(child)})
		case "rest_pattern":
			if child.NamedChildCount() > 0 {
				name := b.nodeText(child.NamedChild(0))
				sym := b.lookupOrDefine(name, symbol.KindVariable)
				pat.Rest = sym
			}
		}
	}
	return pat
}

// hoistTopLevelDecls pre-registers top-level declarations so later declarations
// remain visible to earlier function bodies and module code.
func (b *Builder) hoistTopLevelDecls(root *sitter.Node) {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		switch child.Kind() {
		case "function_declaration":
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				name := b.nodeText(nameNode)
				if b.symtab.LookupLocal(name) == nil {
					b.symtab.Define(name, symbol.KindFunction)
				}
			}
		case "lexical_declaration", "variable_declaration":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				inner := child.NamedChild(j)
				if inner == nil {
					continue
				}
				if nameNode := inner.ChildByFieldName("name"); nameNode != nil && nameNode.Kind() == "identifier" {
					name := b.nodeText(nameNode)
					if b.symtab.LookupLocal(name) == nil {
						b.symtab.Define(name, symbol.KindVariable)
					}
				}
			}
		case "class_declaration":
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				name := b.nodeText(nameNode)
				if b.symtab.LookupLocal(name) == nil {
					b.symtab.Define(name, symbol.KindClass)
				}
			}
		case "enum_declaration":
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				name := b.nodeText(nameNode)
				if b.symtab.LookupLocal(name) == nil {
					b.symtab.Define(name, symbol.KindEnum)
				}
			}
		case "interface_declaration", "type_alias_declaration":
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				name := b.nodeText(nameNode)
				if b.symtab.LookupLocal(name) == nil {
					b.symtab.Define(name, symbol.KindType)
				}
			}
		case "export_statement":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				inner := child.NamedChild(j)
				switch inner.Kind() {
				case "function_declaration":
					if nameNode := inner.ChildByFieldName("name"); nameNode != nil {
						name := b.nodeText(nameNode)
						sym := b.symtab.LookupLocal(name)
						if sym == nil {
							sym = b.symtab.Define(name, symbol.KindFunction)
						}
						sym.Exported = true
					}
				case "lexical_declaration", "variable_declaration":
					for k := uint(0); k < inner.NamedChildCount(); k++ {
						decl := inner.NamedChild(k)
						if decl == nil {
							continue
						}
						if nameNode := decl.ChildByFieldName("name"); nameNode != nil && nameNode.Kind() == "identifier" {
							name := b.nodeText(nameNode)
							sym := b.symtab.LookupLocal(name)
							if sym == nil {
								sym = b.symtab.Define(name, symbol.KindVariable)
							}
							sym.Exported = true
						}
					}
				case "class_declaration":
					if nameNode := inner.ChildByFieldName("name"); nameNode != nil {
						name := b.nodeText(nameNode)
						sym := b.symtab.LookupLocal(name)
						if sym == nil {
							sym = b.symtab.Define(name, symbol.KindClass)
						}
						sym.Exported = true
					}
				case "enum_declaration":
					if nameNode := inner.ChildByFieldName("name"); nameNode != nil {
						name := b.nodeText(nameNode)
						sym := b.symtab.LookupLocal(name)
						if sym == nil {
							sym = b.symtab.Define(name, symbol.KindEnum)
						}
						sym.Exported = true
					}
				case "interface_declaration", "type_alias_declaration":
					if nameNode := inner.ChildByFieldName("name"); nameNode != nil {
						name := b.nodeText(nameNode)
						sym := b.symtab.LookupLocal(name)
						if sym == nil {
							sym = b.symtab.Define(name, symbol.KindType)
						}
						sym.Exported = true
					}
				}
			}
		}
	}
}

// extractComputedPropertyName handles computed property names like
// [(x = new WeakMap(), y = new WeakMap(), methodName)].
// Returns side-effect expressions and the final method name string.
func (b *Builder) extractComputedPropertyName(node *sitter.Node) (sideEffects []Expr, name string) {
	// Walk into parenthesized_expression if present
	inner := node
	for i := uint(0); i < inner.NamedChildCount(); i++ {
		child := inner.NamedChild(i)
		if child.Kind() == "parenthesized_expression" {
			inner = child
			break
		}
		if child.Kind() == "sequence_expression" {
			inner = child
			break
		}
	}

	// Check if inner is a sequence_expression
	if inner.Kind() == "parenthesized_expression" {
		for i := uint(0); i < inner.NamedChildCount(); i++ {
			child := inner.NamedChild(i)
			if child.Kind() == "sequence_expression" {
				inner = child
				break
			}
		}
	}

	if inner.Kind() != "sequence_expression" {
		// Not a sequence — just return the computed name as-is
		// Strip brackets: [expr] → expr text
		text := b.nodeText(node)
		if len(text) >= 2 && text[0] == '[' {
			text = text[1 : len(text)-1]
		}
		return nil, text
	}

	// sequence_expression: expr1, expr2, ..., last
	// All but the last are side effects, last is the method name
	count := inner.NamedChildCount()
	for i := uint(0); i < count; i++ {
		child := inner.NamedChild(i)
		if i < count-1 {
			// Side-effect expression (e.g. x = new WeakMap())
			expr := b.buildExpr(child)
			if expr != nil {
				sideEffects = append(sideEffects, expr)
			}
		} else {
			// Last expression is the method name
			name = b.nodeText(child)
			// Strip quotes if it's a string literal
			if len(name) >= 2 && (name[0] == '\'' || name[0] == '"') {
				name = name[1 : len(name)-1]
			}
		}
	}
	return sideEffects, name
}
