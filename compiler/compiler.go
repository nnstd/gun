package compiler

import "fmt"

// Compile transpiles TypeScript source code to Go source code.
// moduleName is the Go module name (from go.mod) used to resolve relative imports.
// When samePackageImports is true, relative imports are treated as same-package
// references (no Go import generated) — used for flattened node_module packages.
func Compile(source []byte, pkgName, moduleName string, samePackageImports bool) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	transformer := newTransformer(source, pkgName, moduleName, samePackageImports)
	file := transformer.transform(tree.RootNode())

	output, err := emit(file)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return output, nil
}
