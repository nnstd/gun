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
					OriginalName: d.Symbol.OriginalName,
					GoName:       symbol.Capitalize(symbol.Sanitize(d.Symbol.OriginalName)),
					IsJSValue:    true,
				})
			}
		case *hir.VarDecl:
			if d.Exported {
				for _, decl := range d.Declarators {
					if decl.Symbol != nil {
						exports = append(exports, CrossFileExport{
							OriginalName: decl.Symbol.OriginalName,
							GoName:       symbol.Capitalize(symbol.Sanitize(decl.Symbol.OriginalName)),
							IsJSValue:    true,
						})
					}
				}
			}
		case *hir.ClassDecl:
			if d.Symbol != nil && d.Exported {
				exports = append(exports, CrossFileExport{
					OriginalName: d.Symbol.OriginalName,
					GoName:       symbol.Capitalize(symbol.Sanitize(d.Symbol.OriginalName)),
					IsJSValue:    true,
				})
			}
		case *hir.EnumDecl:
			if d.Symbol != nil && d.Exported {
				exports = append(exports, CrossFileExport{
					OriginalName: d.Symbol.OriginalName,
					GoName:       symbol.Capitalize(symbol.Sanitize(d.Symbol.OriginalName)),
					IsJSValue:    true,
				})
			}
		case *hir.ExportDecl:
			if d.Decl != nil && !d.IsDefault {
				subExports := scanExportedDecl(d)
				exports = append(exports, subExports...)
			}
			for _, n := range d.Names {
				exports = append(exports, CrossFileExport{
					OriginalName: n.ExportedName,
					GoName:       symbol.Capitalize(symbol.Sanitize(n.ExportedName)),
					IsJSValue:    true,
				})
			}
			if d.IsDefault {
				exports = append(exports, CrossFileExport{
					OriginalName: "default",
					GoName:       "Default",
					IsJSValue:    true,
				})
			}
		}
	}

	return exports
}

// ScanHIRTopLevelNames extracts Go-level top-level names declared by an HIR module.
// This is used during package compilation to reserve names from sibling files so
// non-exported module-local declarations do not collide after flattening.
func ScanHIRTopLevelNames(mod *hir.Module) []string {
	var names []string
	add := func(sym *symbol.Symbol, exported bool) {
		if sym == nil {
			return
		}
		name := symbol.Sanitize(sym.OriginalName)
		if exported {
			name = symbol.Capitalize(name)
		}
		names = append(names, name)
	}

	var scanDecl func(hir.Decl, bool)
	scanDecl = func(d hir.Decl, forceExported bool) {
		switch d := d.(type) {
		case *hir.FuncDecl:
			add(d.Symbol, forceExported || d.Exported)
		case *hir.VarDecl:
			exported := forceExported || d.Exported
			for _, decl := range d.Declarators {
				add(decl.Symbol, exported)
			}
		case *hir.ClassDecl:
			add(d.Symbol, forceExported || d.Exported)
		case *hir.EnumDecl:
			add(d.Symbol, forceExported || d.Exported)
		case *hir.InterfaceDecl:
			add(d.Symbol, forceExported || d.Exported)
		case *hir.TypeAliasDecl:
			add(d.Symbol, forceExported || d.Exported)
		case *hir.ExportDecl:
			for _, n := range d.Names {
				names = append(names, symbol.Capitalize(symbol.Sanitize(n.ExportedName)))
			}
			if d.IsDefault {
				names = append(names, "Default")
			}
			if d.Decl != nil {
				// Default exports can still emit a named top-level symbol
				// (e.g. `export default function Foo() {}` -> `var Foo`, `var Default = Foo`).
				// Reserve both the default alias and the named declaration.
				scanDecl(d.Decl, true)
			}
		}
	}

	for _, d := range mod.Declarations {
		scanDecl(d, false)
	}
	return names
}

func scanExportedDecl(d *hir.ExportDecl) []CrossFileExport {
	var exports []CrossFileExport
	switch inner := d.Decl.(type) {
	case *hir.FuncDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				OriginalName: inner.Symbol.OriginalName,
				GoName:       symbol.Capitalize(symbol.Sanitize(inner.Symbol.OriginalName)),
				IsJSValue:    true,
			})
		}
	case *hir.VarDecl:
		for _, decl := range inner.Declarators {
			if decl.Symbol != nil {
				exports = append(exports, CrossFileExport{
					OriginalName: decl.Symbol.OriginalName,
					GoName:       symbol.Capitalize(symbol.Sanitize(decl.Symbol.OriginalName)),
					IsJSValue:    true,
				})
			}
		}
	case *hir.ClassDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				OriginalName: inner.Symbol.OriginalName,
				GoName:       symbol.Capitalize(symbol.Sanitize(inner.Symbol.OriginalName)),
				IsJSValue:    true,
			})
		}
	case *hir.EnumDecl:
		if inner.Symbol != nil {
			exports = append(exports, CrossFileExport{
				OriginalName: inner.Symbol.OriginalName,
				GoName:       symbol.Capitalize(symbol.Sanitize(inner.Symbol.OriginalName)),
				IsJSValue:    true,
			})
		}
	}
	return exports
}
