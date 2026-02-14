package main

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func main() {
	source, _ := os.ReadFile(os.Args[1])
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	parser.SetLanguage(lang)
	tree := parser.Parse(source, nil)
	defer tree.Close()
	printNode(tree.RootNode(), source, 0)
}

func printNode(node *sitter.Node, source []byte, depth int) {
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

	fmt.Printf("%s%s%s  %q\n", indent, node.Kind(), fieldName, text)
	for i := uint(0); i < node.ChildCount(); i++ {
		printNode(node.Child(i), source, depth+1)
	}
}
