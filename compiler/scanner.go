package compiler

import (
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ScanExports parses a TypeScript file and returns its exported symbols.
// This is a fast pre-pass that only looks at top-level declarations,
// without performing full transformation.
func ScanExports(source []byte) ([]PackageExport, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	var exports []PackageExport

	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		kind := child.Kind()

		switch kind {
		case "export_statement":
			exports = append(exports, scanExportStatement(child, source)...)

		case "expression_statement":
			// IIFE enum pattern:
			// (function(X) { X["FOO"] = "foo"; })(X || (X = {}));
			if exps := scanIIFEEnum(child, source); len(exps) > 0 {
				exports = append(exports, exps...)
			}

		case "variable_declaration", "lexical_declaration":
			exports = append(exports, scanVarDecl(child, source)...)

		case "function_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name:   name,
					GoName: capitalize(name),
					Kind:   "function",
				})
			}

		case "class_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name:   name,
					GoName: capitalize(name),
					Kind:   "class",
				})
			}

		case "enum_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name:      name,
					GoName:    capitalize(name),
					Kind:      "enum",
					IsJSValue: true,
				})
			}

		case "interface_declaration", "type_alias_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name:   name,
					GoName: capitalize(name),
					Kind:   "type",
				})
			}
		}
	}

	return exports, nil
}

func scanExportStatement(node *sitter.Node, source []byte) []PackageExport {
	var exports []PackageExport
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		kind := child.Kind()

		switch kind {
		case "function_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name: name, GoName: capitalize(name), Kind: "function",
				})
			}
		case "variable_declaration", "lexical_declaration":
			exports = append(exports, scanVarDecl(child, source)...)
		case "class_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name: name, GoName: capitalize(name), Kind: "class",
				})
			}
		case "enum_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name: name, GoName: capitalize(name), Kind: "enum", IsJSValue: true,
				})
			}
		case "interface_declaration", "type_alias_declaration":
			if name := extractDeclName(child, source); name != "" {
				exports = append(exports, PackageExport{
					Name: name, GoName: capitalize(name), Kind: "type",
				})
			}
		default:
			// export default <expression> — check for "default" keyword in parent
			hasDefault := false
			for j := uint(0); j < node.ChildCount(); j++ {
				if node.Child(j).Kind() == "default" {
					hasDefault = true
					break
				}
			}
			if hasDefault {
				exports = append(exports, PackageExport{
					Name: "default", GoName: "Default", Kind: "var", IsJSValue: true,
				})
			}
		case "export_clause":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				spec := child.NamedChild(j)
				if spec.Kind() == "export_specifier" {
					nameNode := spec.ChildByFieldName("name")
					if nameNode != nil {
						name := nameNode.Utf8Text(source)
						exports = append(exports, PackageExport{
							Name: name, GoName: capitalize(name),
							Kind: "var", IsJSValue: true,
						})
					}
				}
			}
		}
	}
	return exports
}

func scanVarDecl(node *sitter.Node, source []byte) []PackageExport {
	var exports []PackageExport
	for i := uint(0); i < node.NamedChildCount(); i++ {
		decl := node.NamedChild(i)
		if decl.Kind() != "variable_declarator" {
			continue
		}
		nameNode := decl.ChildByFieldName("name")
		if nameNode == nil || nameNode.Kind() != "identifier" {
			continue
		}
		name := nameNode.Utf8Text(source)

		valueNode := decl.ChildByFieldName("value")
		isJSValue := true
		kind := "var"
		if valueNode != nil {
			vk := valueNode.Kind()
			if vk == "new_expression" {
				isJSValue = false
			}
			if vk == "arrow_function" || vk == "function" {
				kind = "function"
				isJSValue = false
			}
		}

		exports = append(exports, PackageExport{
			Name: name, GoName: capitalize(name), Kind: kind, IsJSValue: isJSValue,
		})
	}
	return exports
}

func scanIIFEEnum(node *sitter.Node, source []byte) []PackageExport {
	if node.NamedChildCount() == 0 {
		return nil
	}
	expr := node.NamedChild(0)
	if expr.Kind() != "call_expression" {
		return nil
	}
	argsNode := expr.ChildByFieldName("arguments")
	if argsNode == nil || argsNode.NamedChildCount() == 0 {
		return nil
	}
	argText := argsNode.Utf8Text(source)
	if !strings.Contains(argText, "||") || !strings.Contains(argText, "= {}") {
		return nil
	}
	fnNode := expr.ChildByFieldName("function")
	if fnNode == nil {
		return nil
	}
	if fnNode.Kind() == "parenthesized_expression" && fnNode.NamedChildCount() > 0 {
		fnNode = fnNode.NamedChild(0)
	}
	if fnNode.Kind() != "function_expression" && fnNode.Kind() != "arrow_function" {
		return nil
	}
	paramsNode := fnNode.ChildByFieldName("parameters")
	if paramsNode == nil || paramsNode.NamedChildCount() == 0 {
		return nil
	}
	paramNode := paramsNode.NamedChild(0)
	var enumName string
	if paramNode.Kind() == "identifier" {
		enumName = paramNode.Utf8Text(source)
	} else if paramNode.Kind() == "required_parameter" {
		nameNode := paramNode.ChildByFieldName("pattern")
		if nameNode != nil {
			enumName = nameNode.Utf8Text(source)
		}
	}
	if enumName == "" {
		return nil
	}
	return []PackageExport{{
		Name: enumName, GoName: capitalize(enumName), Kind: "enum", IsJSValue: true,
	}}
}

func extractDeclName(node *sitter.Node, source []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	return nameNode.Utf8Text(source)
}
