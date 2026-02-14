package compiler

import (
	"fmt"

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
