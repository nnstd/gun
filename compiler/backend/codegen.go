// Package backend provides Go code generation from HIR.
//
// The backend has two stages:
//   - Lower: converts HIR nodes to go/ast nodes
//   - Generate: formats go/ast as Go source text
package backend

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
)

// Generate formats a Go AST file as source code bytes.
func Generate(file *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	fset.AddFile(file.Name.Name+".go", 1, 1000000)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
