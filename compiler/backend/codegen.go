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
	"go/printer"
	"go/token"
)

// Generate formats a Go AST file as source code bytes.
// Falls back to raw printer if go/format rejects the AST.
func Generate(file *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	fset.AddFile(file.Name.Name+".go", 1, 1000000)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		// Fallback: use raw printer with explicit line breaks
		buf.Reset()
		cfg := &printer.Config{Mode: printer.TabIndent, Tabwidth: 4}
		if err2 := cfg.Fprint(&buf, fset, file); err2 != nil {
			return nil, err2
		}
		return buf.Bytes(), nil
	}
	return buf.Bytes(), nil
}
