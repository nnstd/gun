package compiler

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
)

func emit(file *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	fset.AddFile(file.Name.Name+".go", 1, 1000000)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
