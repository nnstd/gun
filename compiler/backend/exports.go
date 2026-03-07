package backend

import (
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// ScanHIRExports extracts exported symbols from an HIR module.
// This is the pipeline equivalent of compiler.ScanExports().
func ScanHIRExports(mod *hir.Module) []CrossFileExport {
	var exports []CrossFileExport

	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *hir.FuncDecl:
			if d.Symbol != nil && d.Exported {
				exports = append(exports, CrossFileExport{
					GoName:    symbol.Capitalize(d.Symbol.OriginalName),
					IsJSValue: true,
				})
			}
		case *hir.VarDecl:
			if d.Exported {
				for _, decl := range d.Declarators {
					if decl.Symbol != nil {
						exports = append(exports, CrossFileExport{
							GoName:    symbol.Capitalize(decl.Symbol.OriginalName),
							IsJSValue: true,
						})
					}
				}
			}
		case *hir.ClassDecl:
			if d.Symbol != nil && d.Exported {
				exports = append(exports, CrossFileExport{
					GoName:    symbol.Capitalize(d.Symbol.OriginalName),
					IsJSValue: true,
				})
			}
		case *hir.EnumDecl:
			if d.Symbol != nil && d.Exported {
				exports = append(exports, CrossFileExport{
					GoName:    symbol.Capitalize(d.Symbol.OriginalName),
					IsJSValue: true,
				})
			}
		case *hir.ExportDecl:
			if d.Decl != nil {
				subExports := scanExportedDecl(d)
				exports = append(exports, subExports...)
			}
			if d.IsDefault {
				exports = append(exports, CrossFileExport{
					GoName:    "Default",
					IsJSValue: true,
				})
			}
		}
	}

	return exports
}

func scanExportedDecl(d *hir.ExportDecl) []CrossFileExport {
	var exports []CrossFileExport
	switch inner := d.Decl.(type) {
	case *hir.FuncDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				GoName:    symbol.Capitalize(inner.Symbol.OriginalName),
				IsJSValue: true,
			})
		}
	case *hir.VarDecl:
		for _, decl := range inner.Declarators {
			if decl.Symbol != nil {
				exports = append(exports, CrossFileExport{
					GoName:    symbol.Capitalize(decl.Symbol.OriginalName),
					IsJSValue: true,
				})
			}
		}
	case *hir.ClassDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				GoName:    symbol.Capitalize(inner.Symbol.OriginalName),
				IsJSValue: true,
			})
		}
	case *hir.EnumDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				GoName:    symbol.Capitalize(inner.Symbol.OriginalName),
				IsJSValue: true,
			})
		}
	}
	return exports
}
