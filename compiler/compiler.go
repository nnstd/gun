package compiler

import "fmt"

// Compile transpiles TypeScript source code to Go source code.
func Compile(source []byte, pkgName string) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	transformer := newTransformer(source, pkgName)
	file := transformer.transform(tree.RootNode())

	output, err := emit(file)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return output, nil
}
