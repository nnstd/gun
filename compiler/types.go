package compiler

import (
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// mapType converts a tree-sitter TypeScript type annotation node to a go/ast type expression.
func (t *Transformer) mapTypeNode(node *sitter.Node) ast.Expr {
	if node == nil {
		return t.jsValueType()
	}

	kind := node.Kind()
	text := node.Utf8Text(t.source)

	switch kind {
	case "predefined_type":
		return t.mapPredefinedType(text)

	case "type_identifier":
		return ident(text)

	case "array_type":
		// tree-sitter has the element type as the first named child (no field name)
		if node.NamedChildCount() > 0 {
			return sliceType(t.mapTypeNode(node.NamedChild(0)))
		}
		return sliceType(t.jsValueType())

	case "generic_type":
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil {
			return t.jsValueType()
		}
		name := nameNode.Utf8Text(t.source)
		argsNode := node.ChildByFieldName("type_arguments")

		switch name {
		case "Array":
			if argsNode != nil && argsNode.NamedChildCount() > 0 {
				return sliceType(t.mapTypeNode(argsNode.NamedChild(0)))
			}
			return sliceType(t.jsValueType())
		case "Map":
			if argsNode != nil && argsNode.NamedChildCount() >= 2 {
				k := t.mapTypeNode(argsNode.NamedChild(0))
				v := t.mapTypeNode(argsNode.NamedChild(1))
				return mapType(k, v)
			}
			return mapType(ident("string"), t.jsValueType())
		case "Set":
			if argsNode != nil && argsNode.NamedChildCount() > 0 {
				k := t.mapTypeNode(argsNode.NamedChild(0))
				return mapType(k, &ast.StructType{Fields: &ast.FieldList{}})
			}
			return mapType(t.jsValueType(), &ast.StructType{Fields: &ast.FieldList{}})
		case "Record":
			if argsNode != nil && argsNode.NamedChildCount() >= 2 {
				k := t.mapTypeNode(argsNode.NamedChild(0))
				v := t.mapTypeNode(argsNode.NamedChild(1))
				return mapType(k, v)
			}
			return mapType(ident("string"), t.jsValueType())
		case "Promise":
			if argsNode != nil && argsNode.NamedChildCount() > 0 {
				return t.mapTypeNode(argsNode.NamedChild(0))
			}
			return t.jsValueType()
		case "Partial", "Required", "Readonly":
			if argsNode != nil && argsNode.NamedChildCount() > 0 {
				return t.mapTypeNode(argsNode.NamedChild(0))
			}
			return t.jsValueType()
		default:
			return ident(name)
		}

	case "union_type":
		return t.mapUnionType(node)

	case "intersection_type":
		// Use the first type in the intersection
		if node.NamedChildCount() > 0 {
			return t.mapTypeNode(node.NamedChild(0))
		}
		return t.jsValueType()

	case "parenthesized_type":
		if node.NamedChildCount() > 0 {
			return t.mapTypeNode(node.NamedChild(0))
		}
		return t.jsValueType()

	case "object_type":
		return t.mapObjectType(node)

	case "function_type":
		return t.mapFunctionType(node)

	case "tuple_type":
		// Simplify: if all elements same type, use array; otherwise use struct
		if node.NamedChildCount() > 0 {
			first := t.mapTypeNode(node.NamedChild(0))
			return sliceType(first)
		}
		return sliceType(t.jsValueType())

	case "literal_type":
		if node.NamedChildCount() > 0 {
			child := node.NamedChild(0)
			childText := child.Utf8Text(t.source)
			switch child.Kind() {
			case "string":
				return ident("string")
			case "number":
				return ident("float64")
			case "true", "false":
				return ident("bool")
			case "null":
				return t.jsValueType()
			default:
				_ = childText
				return t.jsValueType()
			}
		}
		return t.jsValueType()

	case "type_query":
		// typeof X → just use the type
		return t.jsValueType()

	case "index_type_query":
		return ident("string")

	case "indexed_access_type":
		return t.jsValueType()

	default:
		// Fallback: use the text as-is if it looks like an identifier
		if isSimpleIdent(text) {
			return ident(text)
		}
		return t.jsValueType()
	}
}

func (t *Transformer) mapPredefinedType(text string) ast.Expr {
	switch text {
	case "string":
		return ident("string")
	case "number":
		return ident("float64")
	case "boolean":
		return ident("bool")
	case "void", "undefined", "never":
		return nil // no return type
	case "any", "unknown":
		return t.jsValueType()
	case "null":
		return t.jsValueType()
	case "bigint":
		return ident("int64")
	case "symbol":
		return ident("string")
	case "object":
		return mapType(ident("string"), t.jsValueType())
	default:
		return ident(text)
	}
}

func (t *Transformer) mapUnionType(node *sitter.Node) ast.Expr {
	count := node.NamedChildCount()
	if count == 0 {
		return t.jsValueType()
	}

	// Check for T | null / T | undefined → *T
	var nonNullTypes []ast.Expr
	hasNull := false
	for i := uint(0); i < count; i++ {
		child := node.NamedChild(i)
		text := child.Utf8Text(t.source)
		if text == "null" || text == "undefined" {
			hasNull = true
			continue
		}
		nonNullTypes = append(nonNullTypes, t.mapTypeNode(child))
	}

	if hasNull && len(nonNullTypes) == 1 {
		return ptrType(nonNullTypes[0])
	}
	if len(nonNullTypes) == 1 {
		return nonNullTypes[0]
	}
	if len(nonNullTypes) > 0 {
		return nonNullTypes[0]
	}
	return t.jsValueType()
}

func (t *Transformer) mapObjectType(node *sitter.Node) ast.Expr {
	// Check if it's an index signature → map type
	count := node.NamedChildCount()
	for i := uint(0); i < count; i++ {
		child := node.NamedChild(i)
		if child.Kind() == "index_signature" {
			// map[K]V
			return mapType(ident("string"), t.jsValueType())
		}
	}

	// Otherwise build a struct
	var fields []*ast.Field
	for i := uint(0); i < count; i++ {
		child := node.NamedChild(i)
		if child.Kind() == "property_signature" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")
			if nameNode != nil {
				name := capitalize(nameNode.Utf8Text(t.source))
				var typ ast.Expr = t.jsValueType()
				if typeNode != nil {
					// type_annotation has a child that is the actual type
					if typeNode.Kind() == "type_annotation" && typeNode.NamedChildCount() > 0 {
						typ = t.mapTypeNode(typeNode.NamedChild(0))
					} else {
						typ = t.mapTypeNode(typeNode)
					}
				}
				if typ != nil {
					fields = append(fields, field(name, typ))
				}
			}
		}
	}

	return &ast.StructType{Fields: fieldList(fields...)}
}

func (t *Transformer) mapFunctionType(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
	returnNode := node.ChildByFieldName("return_type")

	var params []*ast.Field
	if paramsNode != nil {
		for i := uint(0); i < paramsNode.NamedChildCount(); i++ {
			p := paramsNode.NamedChild(i)
			if p.Kind() == "required_parameter" || p.Kind() == "optional_parameter" {
				pName := ""
				nameNode := p.ChildByFieldName("pattern")
				if nameNode != nil {
					pName = nameNode.Utf8Text(t.source)
				}
				var pType ast.Expr = t.jsValueType()
				typeNode := p.ChildByFieldName("type")
				if typeNode != nil && typeNode.NamedChildCount() > 0 {
					pType = t.mapTypeNode(typeNode.NamedChild(0))
				}
				if pType != nil {
					params = append(params, field(pName, pType))
				}
			}
		}
	}

	var results []*ast.Field
	if returnNode != nil {
		retType := t.mapTypeNode(returnNode)
		if retType != nil {
			results = append(results, field("", retType))
		}
	}

	ft := &ast.FuncType{
		Params: fieldList(params...),
	}
	if len(results) > 0 {
		ft.Results = fieldList(results...)
	}
	return ft
}

// getTypeAnnotation extracts the type from a type_annotation node.
func (t *Transformer) getTypeAnnotation(node *sitter.Node) ast.Expr {
	if node == nil {
		return nil
	}
	if node.Kind() == "type_annotation" && node.NamedChildCount() > 0 {
		return t.mapTypeNode(node.NamedChild(0))
	}
	return t.mapTypeNode(node)
}

func isSimpleIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if i == 0 && !isLetter(c) {
			return false
		}
		if !isLetter(c) && !isDigit(c) {
			return false
		}
	}
	return true
}

func isLetter(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

// isIntegerLiteral checks if a number string represents an integer.
func isIntegerLiteral(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	// Hex
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return true
	}
	// Octal
	if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
		return true
	}
	// Binary
	if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
		return true
	}
	for _, c := range s {
		if c == '.' || c == 'e' || c == 'E' {
			return false
		}
	}
	return true
}

// numberTokenKind returns the appropriate token kind for a number literal.
func numberTokenKind(s string) token.Token {
	if isIntegerLiteral(s) {
		return token.INT
	}
	return token.FLOAT
}
