package compiler

import "fmt"

// PackageExport describes a symbol exported from a file within the same package.
type PackageExport struct {
	Name      string // original TS name (e.g. "DefaultValuesForTypeKey")
	GoName    string // capitalized Go name (e.g. "DefaultValuesForTypeKey")
	Kind      string // "var", "function", "class", "enum", "type"
	IsJSValue bool   // true if the export is a *jsvalue.JSValue variable
}

// PackageExports maps filename → list of exports from that file.
type PackageExports map[string][]PackageExport

// Compile transpiles TypeScript source code to Go source code.
// moduleName is the Go module name (from go.mod) used to resolve relative imports.
// When samePackageImports is true, relative imports are treated as same-package
// references (no Go import generated) — used for flattened node_module packages.
func Compile(source []byte, pkgName, moduleName string, samePackageImports bool) ([]byte, error) {
	return CompileWithExports(source, pkgName, moduleName, samePackageImports, nil)
}

// CompileWithExports transpiles TypeScript source with knowledge of cross-file exports.
// exports provides metadata about symbols exported from other files in the same package.
func CompileWithExports(source []byte, pkgName, moduleName string, samePackageImports bool, exports PackageExports) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	transformer := newTransformer(source, pkgName, moduleName, samePackageImports)

	// Pre-populate transformer with cross-file export knowledge
	if exports != nil {
		for _, fileExports := range exports {
			for _, exp := range fileExports {
				switch exp.Kind {
				case "var", "enum":
					transformer.pkgVarTyped[exp.GoName] = !exp.IsJSValue
				case "function":
					// Cross-file functions are known to exist; track param count as 0
					// so they're recognized as hoisted functions (not JSValue variables).
					if _, exists := transformer.funcParamCounts[exp.GoName]; !exists {
						transformer.funcParamCounts[exp.GoName] = 0
					}
				}
			}
		}
	}

	file := transformer.transform(tree.RootNode())

	output, err := emit(file)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return output, nil
}

// CompilePackage transpiles multiple TypeScript files that belong to the
// same Go package. It scans all files for exports first, then compiles
// each file with cross-file export knowledge.
func CompilePackage(files map[string][]byte, pkgName, moduleName string) (map[string][]byte, error) {
	// Phase 1: Scan all files for exports
	exports := make(PackageExports)
	for name, source := range files {
		exps, err := ScanExports(source)
		if err != nil {
			continue // skip files that fail to parse
		}
		exports[name] = exps
	}

	// Phase 2: Compile each file with cross-file knowledge
	results := make(map[string][]byte)
	for name, source := range files {
		out, err := CompileWithExports(source, pkgName, moduleName, true, exports)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		results[name] = out
	}
	return results, nil
}
