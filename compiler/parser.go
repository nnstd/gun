package compiler

import (
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func parseTypeScript(source []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("set language: %w", err)
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse TypeScript source")
	}

	root := tree.RootNode()
	if root.HasError() {
		return tree, fmt.Errorf("parse tree contains errors")
	}

	return tree, nil
}

// DumpAST parses TypeScript source and returns a human-readable AST dump.
func DumpAST(source []byte) (string, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return "", err
	}
	defer tree.Close()

	var buf strings.Builder
	printNode(&buf, tree.RootNode(), source, 0)
	return buf.String(), nil
}

func printNode(buf *strings.Builder, node *sitter.Node, source []byte, depth int) {
	indent := strings.Repeat("  ", depth)
	text := node.Utf8Text(source)
	if len(text) > 60 {
		text = text[:60] + "..."
	}
	text = strings.ReplaceAll(text, "\n", "\\n")

	fieldName := ""
	if node.Parent() != nil {
		for i := uint(0); i < node.Parent().ChildCount(); i++ {
			child := node.Parent().Child(i)
			if child.Id() == node.Id() {
				fn := node.Parent().FieldNameForChild(uint32(i))
				if fn != "" {
					fieldName = " [field:" + fn + "]"
				}
				break
			}
		}
	}

	fmt.Fprintf(buf, "%s%s%s  %q\n", indent, node.Kind(), fieldName, text)
	for i := uint(0); i < node.ChildCount(); i++ {
		printNode(buf, node.Child(i), source, depth+1)
	}
}
