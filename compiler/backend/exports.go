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
				// CJS module.exports = { X, Y } → extract named exports,
				// but only when the value is a local identifier (shorthand
				// or explicit ref). Skip inline values and imports that
				// have no backing Go variable.
				if d.Decl != nil {
					if vd, ok := d.Decl.(*hir.VarDecl); ok {
						for _, dec := range vd.Declarators {
							if obj, ok := dec.Init.(*hir.ObjectLiteral); ok {
								for _, prop := range obj.Properties {
									if prop.KeyName == "" || prop.Computed {
										continue
									}
									if _, ok := prop.Value.(*hir.Identifier); !ok {
										continue
									}
									exports = append(exports, CrossFileExport{
										OriginalName: prop.KeyName,
										GoName:       symbol.Capitalize(symbol.Sanitize(prop.KeyName)),
										IsJSValue:    true,
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return exports
}

// ScanHIRCJSExports detects CommonJS-style exports.X = ... assignments
// and returns them as CrossFileExports so the pipeline can wire cross-file
// import resolution for modules that use CJS export patterns.
func ScanHIRCJSExports(mod *hir.Module) []CrossFileExport {
	var exports []CrossFileExport
	seen := map[string]bool{}
	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *hir.VarDecl:
			for _, dec := range d.Declarators {
				if dec.Init != nil {
					collectCJSExports(dec.Init, seen, &exports)
				}
			}
		case *hir.TopLevelStmt:
			es, ok := d.Stmt.(*hir.ExprStmt)
			if !ok {
				continue
			}
			assign, ok := es.Expr.(*hir.AssignExpr)
			if !ok {
				continue
			}
			// Only extract named exports from module.exports = { ... }
			// in expression statements. The exports.X = value pattern
			// in expression statements has no backing Go variable.
			mem, ok := assign.Left.(*hir.MemberExpr)
			if !ok {
				continue
			}
			id, ok := mem.Object.(*hir.Identifier)
			if !ok || id.Name != "module" || mem.Property != "exports" {
				continue
			}
			collectCJSExportsFromAssign(assign, seen, &exports)
		}
	}
	return exports
}

func collectCJSExports(expr hir.Expr, seen map[string]bool, exports *[]CrossFileExport) {
	assign, ok := expr.(*hir.AssignExpr)
	if !ok {
		return
	}
	collectCJSExportsFromAssign(assign, seen, exports)
}

func collectCJSExportsFromAssign(assign *hir.AssignExpr, seen map[string]bool, exports *[]CrossFileExport) {
	mem, ok := assign.Left.(*hir.MemberExpr)
	if !ok {
		return
	}
	id, ok := mem.Object.(*hir.Identifier)
	if !ok {
		return
	}

	// module.exports = { X, Y, Z } → extract named exports from object literal
	if id.Name == "module" && mem.Property == "exports" {
		if obj, ok := assign.Right.(*hir.ObjectLiteral); ok {
			for _, prop := range obj.Properties {
				name := prop.KeyName
				if name == "" || seen[name] || prop.Computed {
					continue
				}
				// Only extract if value is a local identifier reference
				if _, ok := prop.Value.(*hir.Identifier); !ok {
					continue
				}
				seen[name] = true
				*exports = append(*exports, CrossFileExport{
					OriginalName: name,
					GoName:       symbol.Capitalize(symbol.Sanitize(name)),
					IsJSValue:    true,
				})
			}
		}
		return
	}

	// exports.X = value → named export
	if id.Name == "exports" {
		propName := mem.Property
		if propName == "" || seen[propName] {
			return
		}
		seen[propName] = true
		*exports = append(*exports, CrossFileExport{
			OriginalName: propName,
			GoName:       symbol.Capitalize(symbol.Sanitize(propName)),
			IsJSValue:    true,
		})
	}
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
