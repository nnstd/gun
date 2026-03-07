package compiler

import (
	"fmt"
	"path/filepath"
	"strings"

	tcontext "github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/pipeline"
)

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
	return CompileWithExports(source, pkgName, moduleName, "", samePackageImports, nil)
}

// CompileWithExports transpiles TypeScript source with knowledge of cross-file exports.
// exports provides metadata about symbols exported from other files in the same package.
func CompileWithExports(source []byte, pkgName, moduleName, currentFile string, samePackageImports bool, exports PackageExports) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	transformer := newTransformer(source, pkgName, moduleName, samePackageImports)

	// Pre-populate transformer with cross-file export knowledge
	// (exclude the current file's own exports to avoid self-conflicts)
	for fileName, fileExports := range exports {
		if fileName == currentFile {
			continue
		}
		for _, exp := range fileExports {
			// All cross-file exports are JSValue in all-JSValue architecture
			transformer.pkgVarTyped[exp.GoName] = false
			transformer.crossFileExports[exp.GoName] = true
		}
	}

	file := transformer.transform(tree.RootNode())

	output, err := emit(file)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return output, nil
}

// CompileNewPipeline transpiles TypeScript source using the new multi-stage pipeline
// (HIR → MIR → SSA → Passes → Backend). This is the experimental path that uses
// hygienic symbol IDs and the full optimization pipeline.
func CompileNewPipeline(source []byte, pkgName, moduleName string, optLevel int) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	ctx := tcontext.New()
	RegisterDefaultBuiltins(ctx)

	level := pipeline.OptLevel(optLevel)
	p := pipeline.NewWithContext(level, ctx)

	return p.CompileTree(tree.RootNode(), source, pkgName, moduleName, false)
}

// CompilePackage transpiles multiple TypeScript files that belong to the
// same Go package. It scans all files for exports first, then compiles
// each file with cross-file export knowledge.
func CompilePackage(files map[string][]byte, pkgName, moduleName, entryFile string) (map[string][]byte, error) {
	// For the entry file's "export default", other files' default imports
	// resolve to the entry's "Default". But non-entry files also have
	// "export default" which becomes "Default". To avoid conflicts,
	// rename non-entry file defaults by rewriting the source before compilation:
	// `export default X` → `export const _fileDefault = X` in non-entry files.
	// Then update imports of non-entry defaults to use the new name.
	// For now, handled by post-compilation rename + reference fix.
	// Phase 1: Scan all files for exports
	exports := make(PackageExports)
	for name, source := range files {
		exps, err := ScanExports(source)
		if err != nil {
			continue // skip files that fail to parse
		}
		exports[name] = exps
	}

	// Check for conflicting Default exports across files.
	// Only the entry file's Default is kept; others are renamed to file-specific names.
	defaultFiles := []string{}
	for name, fileExports := range exports {
		for _, exp := range fileExports {
			if exp.GoName == "Default" {
				defaultFiles = append(defaultFiles, name)
			}
		}
	}
	renameDefault := make(map[string]bool)
	if len(defaultFiles) > 1 {
		for _, name := range defaultFiles {
			if name != entryFile {
				renameDefault[name] = true
			}
		}
	}

	// Phase 2: Compile each file with cross-file knowledge
	results := make(map[string][]byte)
	for name, source := range files {
		out, err := CompileWithExports(source, pkgName, moduleName, name, true, exports)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		// Rename Default in non-entry files to avoid conflicts
		if renameDefault[name] {
			out = renameDefaultExport(out, name)
		}
		results[name] = out
	}
	return results, nil
}

// renameDefaultExport replaces "Default" declarations in compiled output
// with a file-specific name to avoid conflicts between multiple files.
func renameDefaultExport(source []byte, fileName string) []byte {
	// Generate a file-specific name from the filename
	base := filepath.Base(fileName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	// Sanitize: replace non-alphanumeric with underscore, capitalize
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
	name = strings.ToUpper(name[:1]) + name[1:] + "Default"

	// Replace "var Default " and "Default =" at line starts
	s := string(source)
	s = strings.ReplaceAll(s, "var Default ", "var "+name+" ")
	s = strings.ReplaceAll(s, "var Default\n", "var "+name+"\n")
	return []byte(s)
}
