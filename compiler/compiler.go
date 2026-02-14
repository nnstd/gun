package compiler

import "fmt"

// Compile transpiles TypeScript source code to Go source code.
// moduleName is the Go module name (from go.mod) used to resolve relative imports.
func Compile(source []byte, pkgName, moduleName string) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	transformer := newTransformer(source, pkgName, moduleName)
	file := transformer.transform(tree.RootNode())

	output, err := emit(file)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return output, nil
}
