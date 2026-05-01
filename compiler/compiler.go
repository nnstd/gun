package compiler

import (
	"fmt"

	"github.com/nnstd/gun/compiler/backend"
	tcontext "github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
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
	return CompileWithOptLevel(source, pkgName, moduleName, samePackageImports, 0)
}

// CompileWithOptLevel transpiles TypeScript source code using the multi-stage
// pipeline at the requested optimization level.
func CompileWithOptLevel(source []byte, pkgName, moduleName string, samePackageImports bool, optLevel int) ([]byte, error) {
	return CompileWithOptLevelAndPath(source, pkgName, moduleName, "", samePackageImports, optLevel)
}

// CompileWithContext transpiles TypeScript source using a pre-built context.
// Use this when you need to augment the context with additional builtins
// (e.g., test262 harness functions) before compilation.
func CompileWithContext(source []byte, pkgName, moduleName string, samePackageImports bool, optLevel int, ctx *tcontext.TranspilerContext) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	p := pipeline.NewWithContext(tcontext.OptLevel(optLevel), ctx)
	return p.CompileTreeWithPathOptions(tree.RootNode(), source, pkgName, moduleName, "", samePackageImports, nil)
}

func CompileWithOptLevelAndPath(source []byte, pkgName, moduleName, sourcePath string, samePackageImports bool, optLevel int) ([]byte, error) {
	return CompileWithOptLevelAndPathOptions(source, pkgName, moduleName, sourcePath, samePackageImports, optLevel, nil)
}

func CompileWithOptLevelAndPathOptions(source []byte, pkgName, moduleName, sourcePath string, samePackageImports bool, optLevel int, opts *CompileOptions) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	p := newDefaultPipeline(optLevel)
	var lowerCfg *backend.LowerConfig
	if opts != nil {
		lowerCfg = &backend.LowerConfig{
			CPUProfile: opts.CPUProfile,
			Otel:       opts.Otel,
		}
	}
	return p.CompileTreeWithPathOptions(tree.RootNode(), source, pkgName, moduleName, sourcePath, samePackageImports, lowerCfg)
}

// CompileWithExports transpiles TypeScript source with knowledge of cross-file exports.
// exports provides metadata about symbols exported from other files in the same package.
func CompileWithExports(source []byte, pkgName, moduleName, currentFile string, samePackageImports bool, exports PackageExports) ([]byte, error) {
	tree, err := parseTypeScript(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Close()

	hirMod := hir.BuildModuleWithPath(tree.RootNode(), source, pkgName, currentFile)
	crossFileExports, reservedNames := compileWithExportsMetadata(exports, currentFile)
	p := newDefaultPipeline(0)
	return p.CompileHIRWithExports(hirMod, moduleName, samePackageImports, crossFileExports, reservedNames, nil, nil, nil, "", nil)
}

// CompileNewPipeline transpiles TypeScript source using the pipeline at the
// requested optimization level. It remains as a compatibility alias.
func CompileNewPipeline(source []byte, pkgName, moduleName string, optLevel int) ([]byte, error) {
	return CompileWithOptLevel(source, pkgName, moduleName, false, optLevel)
}

// CompilePackage transpiles multiple TypeScript files that belong to the
// same Go package using the default pipeline configuration.
func CompilePackage(files map[string][]byte, pkgName, moduleName, entryFile string) (map[string][]byte, error) {
	return CompilePackageWithOptLevel(files, pkgName, moduleName, entryFile, 0)
}

// CompilePackageWithOptLevel transpiles multiple TypeScript files that belong to
// the same Go package using the multi-stage pipeline.
func CompilePackageWithOptLevel(files map[string][]byte, pkgName, moduleName, entryFile string, optLevel int) (map[string][]byte, error) {
	return CompilePackageWithOptLevelOptions(files, pkgName, moduleName, entryFile, optLevel, nil)
}

func CompilePackageWithOptLevelOptions(files map[string][]byte, pkgName, moduleName, entryFile string, optLevel int, opts *CompileOptions) (map[string][]byte, error) {
	p := newDefaultPipeline(optLevel)
	var lowerCfg *backend.LowerConfig
	if opts != nil {
		lowerCfg = &backend.LowerConfig{
			CPUProfile: opts.CPUProfile,
			Otel:       opts.Otel,
		}
	}
	return p.CompilePackageWithOptions(files, pkgName, moduleName, entryFile, lowerCfg)
}

func newDefaultPipeline(optLevel int) *pipeline.Pipeline {
	return pipeline.NewWithContext(tcontext.OptLevel(optLevel), NewDefaultContext())
}

// NewDefaultContext creates a TranspilerContext with all default builtins registered.
// Use this when you need to augment the context before passing it to the pipeline.
func NewDefaultContext() *tcontext.TranspilerContext {
	ctx := tcontext.New()
	RegisterDefaultBuiltins(ctx)
	return ctx
}

func compileWithExportsMetadata(exports PackageExports, currentFile string) ([]backend.CrossFileExport, []string) {
	var crossFileExports []backend.CrossFileExport
	var reservedNames []string
	for fileName, fileExports := range exports {
		if fileName == currentFile {
			continue
		}
		for _, exp := range fileExports {
			crossFileExports = append(crossFileExports, backend.CrossFileExport{
				OriginalName: exp.Name,
				GoName:       exp.GoName,
				IsJSValue:    exp.IsJSValue,
			})
			if exp.GoName != "" {
				reservedNames = append(reservedNames, exp.GoName)
			}
		}
	}
	return crossFileExports, reservedNames
}
