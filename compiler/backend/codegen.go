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
	"strings"
)

// Generate formats a Go AST file as source code bytes.
// Falls back to raw printer if go/format rejects the AST.
func Generate(file *ast.File) ([]byte, error) {
	return GenerateWithSource(file, "", 0)
}

// GenerateWithSource formats a Go AST file and preserves original source
// positions when a source path is available.
func GenerateWithSource(file *ast.File, sourcePath string, sourceSize int) ([]byte, error) {
	fset := token.NewFileSet()
	fileName := file.Name.Name + ".go"
	fileSize := 1000000
	if sourcePath != "" && sourceSize > 0 {
		fileSize = sourceSize + sourcePosLineStride
	}
	if maxPos := maxASTPos(file); maxPos > fileSize {
		fileSize = maxPos + sourcePosLineStride
	}
	fset.AddFile(fileName, 1, fileSize)

	var buf bytes.Buffer
	cfg := &printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 4}
	if sourcePath != "" {
		if err := cfg.Fprint(&buf, fset, file); err != nil {
			return nil, err
		}
		return rewriteLineMarkers(buf.Bytes()), nil
	}

	if err := format.Node(&buf, fset, file); err != nil {
		// Fallback: use raw printer with explicit line breaks
		buf.Reset()
		fallback := &printer.Config{Mode: printer.TabIndent, Tabwidth: 4}
		if err2 := fallback.Fprint(&buf, fset, file); err2 != nil {
			return nil, err2
		}
		return buf.Bytes(), nil
	}
	return buf.Bytes(), nil
}

func maxASTPos(file *ast.File) int {
	maxPos := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if pos := int(node.End()); pos > maxPos {
			maxPos = pos
		}
		return true
	})
	return maxPos
}

func rewriteLineMarkers(src []byte) []byte {
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, lineMarkerFunc+"(") || !strings.HasSuffix(trimmed, ")") {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, lineMarkerFunc+"("), ")")
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) != 2 {
			continue
		}
		path := strings.TrimSpace(parts[0])
		lineNo := strings.TrimSpace(parts[1])
		path = strings.Trim(path, `"`)
		lines[i] = "//line " + path + ":" + lineNo
	}
	return []byte(strings.Join(lines, "\n"))
}
