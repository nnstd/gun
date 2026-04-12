package compiler

import (
	"go/ast"
	"path"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// resolvedImport describes how a TS imported name maps to Go.
type resolvedImport struct {
	goImportPath string // Go import path (e.g. "os", "path/filepath")
	goPkgName    string // Go package identifier (e.g. "os", "filepath")
	goSymbol     string // Go symbol name (e.g. "ReadFile"); empty for namespace imports
	isTranspiled bool   // true when the module is transpiled from source (not a known/runtime module)
	useAsJSValue bool   // true when access should go through pkg.AsJSValue / .Get(...)
	jsExportName string // JS export/property name when useAsJSValue is true
}

// moduleMapping maps a TS module to a Go package.
type moduleMapping struct {
	goPath       string // Go import path
	goName       string // Go package name used in code
	useAsJSValue bool   // module is accessed as pkg.AsJSValue
}

// knownModules maps Node.js / TS built-in module names to Go packages.
var knownModules = map[string]moduleMapping{}

// knownSymbols maps (module, symbol) pairs to specific Go translations.
// Only non-standard mappings are stored here — symbols that just need capitalize()
// are handled by the default fallback in processNamedImports.
var knownSymbols = map[string]map[string]resolvedImport{}

// registerModule registers a known Node.js module with its Go package mapping.
// symbolOverrides contains only non-standard translations where capitalize(tsName)
// isn't sufficient. Use "" as the Go symbol value to create a namespace import.
func registerModule(tsModule, goPath, goName string, useAsJSValue bool, symbolOverrides map[string]string) {
	knownModules[tsModule] = moduleMapping{goPath: goPath, goName: goName, useAsJSValue: useAsJSValue}
	if len(symbolOverrides) > 0 {
		syms := make(map[string]resolvedImport, len(symbolOverrides))
		for tsName, goSymbol := range symbolOverrides {
			syms[tsName] = resolvedImport{
				goImportPath: goPath,
				goPkgName:    goName,
				goSymbol:     goSymbol,
			}
		}
		knownSymbols[tsModule] = syms
	}
}

func init() {
	// --- gun runtime modules ---

	registerModule("fs", "github.com/nnstd/gun/runtime/fs", "fs", true, nil)
	registerModule("path", "github.com/nnstd/gun/runtime/path", "nodepath", true, nil)
	registerModule("os", "github.com/nnstd/gun/runtime/os", "nodeos", true, nil)

	// --- Go stdlib mappings ---

	registerModule("http", "net/http", "http", false, nil)
	registerModule("https", "net/http", "http", false, nil)
	registerModule("url", "github.com/nnstd/gun/runtime/url", "url", true, nil)
	registerModule("util", "fmt", "fmt", false, map[string]string{
		"format":  "Sprintf",
		"inspect": "Sprint",
	})
	registerModule("events", "sync", "sync", false, nil)
	registerModule("stream", "io", "io", false, nil)
	registerModule("buffer", "bytes", "bytes", false, nil)
	registerModule("crypto", "crypto", "crypto", false, nil)
	registerModule("child_process", "os/exec", "exec", false, map[string]string{
		"exec":     "Command",
		"execSync": "Command",
		"spawn":    "Command",
	})
	registerModule("assert", "github.com/nnstd/gun/runtime/assert", "assert", true, nil)
	registerModule("module", "github.com/nnstd/gun/runtime/module", "module", true, nil)
}

func (t *Transformer) transformImport(node *sitter.Node) {
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return
	}

	// Extract module path from the string literal
	modulePath := sourceNode.Utf8Text(t.source)
	modulePath = strings.Trim(modulePath, "'\"")

	// Strip node: prefix (e.g. "node:fs" → "fs")
	modulePath = strings.TrimPrefix(modulePath, "node:")

	// Check for type-only import
	typeOnly := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "type" {
			typeOnly = true
			break
		}
	}

	// Find the import_clause
	var clauseNode *sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "import_clause" {
			clauseNode = child
			break
		}
	}
	if clauseNode == nil {
		return
	}

	// Resolve the Go module mapping
	mod, isKnown := knownModules[modulePath]
	if !isKnown {
		mod = t.resolveModulePath(modulePath)
	}
	useAsJSValue := isKnown && mod.useAsJSValue

	// Walk the import clause to extract names
	for i := uint(0); i < clauseNode.NamedChildCount(); i++ {
		child := clauseNode.NamedChild(i)
		switch child.Kind() {
		case "named_imports":
			t.processNamedImports(child, modulePath, mod, typeOnly, !isKnown)

		case "namespace_import":
			// import * as X from "mod" → X becomes a package alias
			var alias string
			for j := uint(0); j < child.NamedChildCount(); j++ {
				if child.NamedChild(j).Kind() == "identifier" {
					alias = child.NamedChild(j).Utf8Text(t.source)
				}
			}
			if alias != "" {
				t.importedNames[alias] = resolvedImport{
					goImportPath: mod.goPath,
					goPkgName:    mod.goName,
					isTranspiled: !isKnown,
					useAsJSValue: useAsJSValue,
				}
			}

		case "identifier":
			// import X from "mod" → default import
			localName := child.Utf8Text(t.source)

			ri := resolvedImport{
				goImportPath: mod.goPath,
				goPkgName:    mod.goName,
				isTranspiled: !isKnown,
				useAsJSValue: useAsJSValue,
			}
			if useAsJSValue {
				t.importedNames[localName] = ri
				continue
			}
			// Check for explicit default symbol mapping in knownSymbols
			if symTable := knownSymbols[modulePath]; symTable != nil {
				if sym, ok := symTable["default"]; ok {
					ri.goSymbol = sym.goSymbol
				}
			}
			// For third-party/relative packages, default import maps to the Default symbol
			if ri.goSymbol == "" && !isKnown {
				if t.samePackageImports && isRelativeImport(modulePath) && !strings.HasPrefix(modulePath, "..") {
					// Same-package imports (not parent-dir): non-entry files have their
					// Default renamed to file-specific names (e.g. EsmDefault).
					ri.goSymbol = fileSpecificDefaultName(modulePath)
				} else {
					ri.goSymbol = "Default"
				}
			}
			t.importedNames[localName] = ri
		}
	}
}

func (t *Transformer) processNamedImports(node *sitter.Node, modulePath string, mod moduleMapping, typeOnly bool, isTranspiled bool) {
	symTable := knownSymbols[modulePath]
	useAsJSValue := !isTranspiled && mod.useAsJSValue

	for i := uint(0); i < node.NamedChildCount(); i++ {
		spec := node.NamedChild(i)
		if spec.Kind() != "import_specifier" {
			continue
		}

		nameNode := spec.ChildByFieldName("name")
		aliasNode := spec.ChildByFieldName("alias")
		if nameNode == nil {
			continue
		}

		origName := nameNode.Utf8Text(t.source)
		localName := origName
		if aliasNode != nil {
			localName = aliasNode.Utf8Text(t.source)
		}

		if useAsJSValue {
			t.importedNames[localName] = resolvedImport{
				goImportPath: mod.goPath,
				goPkgName:    mod.goName,
				isTranspiled: false,
				useAsJSValue: true,
				jsExportName: origName,
			}
			if typeOnly {
				t.importedNames[capitalize(localName)] = resolvedImport{
					goImportPath: mod.goPath,
					goPkgName:    mod.goName,
					isTranspiled: false,
					useAsJSValue: true,
					jsExportName: origName,
				}
			}
			continue
		}

		// Check for a specific symbol mapping
		if symTable != nil {
			if sym, ok := symTable[origName]; ok {
				t.importedNames[localName] = sym
				continue
			}
		}

		// Default: capitalize the symbol and use the module's Go package
		t.importedNames[localName] = resolvedImport{
			goImportPath: mod.goPath,
			goPkgName:    mod.goName,
			goSymbol:     capitalize(origName),
			isTranspiled: isTranspiled,
		}

		// For type-only imports, also register the capitalized form
		if typeOnly {
			t.importedNames[capitalize(localName)] = resolvedImport{
				goImportPath: mod.goPath,
				goPkgName:    mod.goName,
				goSymbol:     capitalize(origName),
			}
		}
	}
}

// SanitizeGoPkgName converts an npm package name to a valid Go package name.
// e.g. "temp-dir" → "temp_dir", "@scope/pkg" → "scope_pkg"
func SanitizeGoPkgName(npmName string) string {
	name := strings.TrimPrefix(npmName, "@")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

// fileSpecificDefaultName generates the renamed default export name for a given
// module path, matching the renameDefaultExport logic in compiler.go.
// e.g. "./lib/platform-shims/esm.mjs" → "EsmDefault"
func fileSpecificDefaultName(modulePath string) string {
	base := path.Base(modulePath)
	base = strings.TrimSuffix(base, path.Ext(base))
	name := ""
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			name += string(r)
		} else {
			name += "_"
		}
	}
	if name == "" {
		name = "FileDefault"
	}
	return strings.ToUpper(name[:1]) + name[1:] + "Default"
}

// isRelativeImport returns true if the module path is a relative import.
func isRelativeImport(modulePath string) bool {
	return strings.HasPrefix(modulePath, ".")
}

// IsKnownModule reports whether the given module name is a built-in polyfilled module.
func IsKnownModule(name string) bool {
	name = strings.TrimPrefix(name, "node:")
	_, ok := knownModules[name]
	return ok
}

// resolveModulePath converts a TS module path to a Go import path.
func (t *Transformer) resolveModulePath(modulePath string) moduleMapping {
	// Relative import: ./foo or ../bar
	if strings.HasPrefix(modulePath, ".") {
		// Same-package mode: relative imports are within the same Go package
		if t.samePackageImports {
			return moduleMapping{goPath: "", goName: ""}
		}
		// Strip file extension
		clean := strings.TrimSuffix(modulePath, ".ts")
		clean = strings.TrimSuffix(clean, ".js")
		clean = strings.TrimSuffix(clean, ".mjs")
		// Use the last path segment as the Go package name
		pkgName := path.Base(clean)
		// Build Go import path: module/relative/path
		modName := t.moduleName
		if modName == "" {
			modName = "main"
		}
		goPath := path.Clean(modName + "/" + strings.TrimPrefix(clean, "./"))
		return moduleMapping{goPath: goPath, goName: pkgName}
	}

	// Third-party package (including scoped): generate module-relative import path
	pkgName := SanitizeGoPkgName(modulePath)
	if t.moduleName != "" {
		return moduleMapping{goPath: t.moduleName + "/" + pkgName, goName: pkgName}
	}
	return moduleMapping{goPath: pkgName, goName: pkgName}
}

// resolveIdentifier checks if a name was imported from a TS module and returns
// the appropriate Go expression. Falls back to builtin identifier mapping.
func (t *Transformer) resolveIdentifier(name string) ast.Expr {
	// JS `arguments` keyword → JSValue array of function arguments.
	// In class methods, _args[0] is `this`, so arguments = _args[1:].
	// In regular functions, arguments = _args.
	if name == "arguments" {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		var argsSlice ast.Expr
		if t.inClassMethod {
			argsSlice = &ast.SliceExpr{X: ident("_args"), Low: intLit("1")}
		} else {
			argsSlice = ident("_args")
		}
		call := callExpr(selectorExpr(ident("jsvalue"), "NewArray"), argsSlice)
		// Set Ellipsis to spread the slice: jsvalue.NewArray(_args[1:]...)
		call.Ellipsis = 1
		return call
	}
	// Local parameters/variables shadow imports.
	// Check both raw name and sanitized name (e.g. "string" → "string_").
	sanitized := sanitizeIdent(name)
	if t.isLocalName(name) || t.isLocalName(sanitized) {
		return ident(sanitized)
	}
	// Exported package-level variables are capitalized in Go declarations.
	// Use goNameRemap for collision-safe names.
	if t.exportedNames[name] {
		return ident(t.resolveGoName(name))
	}
	if imp, ok := t.importedNames[name]; ok {
		if imp.goImportPath != "" {
			// Use aliased import when package name differs from import path's last segment
			if imp.goPkgName != "" && imp.goPkgName != filepath.Base(imp.goImportPath) {
				t.addAliasedImport(imp.goImportPath, imp.goPkgName)
			} else {
				t.addImport(imp.goImportPath)
			}
		}
		if imp.useAsJSValue {
			moduleObj := selectorExpr(ident(imp.goPkgName), "AsJSValue")
			if imp.jsExportName == "" {
				return moduleObj
			}
			return callExpr(selectorExpr(moduleObj, "Get"), stringLit(imp.jsExportName))
		}
		// Namespace import (import * as X) — return the package ident
		if imp.goSymbol == "" {
			if imp.goPkgName == "" {
				// Same-package namespace: return local name as-is (member access handles the rest)
				return ident(name)
			}
			return ident(imp.goPkgName)
		}
		// Same-package reference — no package prefix needed
		if imp.goPkgName == "" {
			return ident(imp.goSymbol)
		}
		// Named import — return pkg.Symbol
		return selectorExpr(ident(imp.goPkgName), imp.goSymbol)
	}
	// Try context-registered identifiers first, fall back to mapIdentifier
	if expr := t.ctx.TransformIdentifier(name, t); expr != nil {
		return expr
	}
	return ident(sanitizeIdent(name))
}
