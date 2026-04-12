// Package pipeline orchestrates the compiler stages from TypeScript source
// through HIR, MIR, SSA, optimization passes, and backend code generation.
//
// It provides configurable optimization levels (O0, O1, O2) and
// deterministic pass ordering. No business logic — only coordination.
package pipeline

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler/backend"
	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/passes"
	"github.com/nnstd/gun/compiler/ssa"
	"github.com/nnstd/gun/compiler/symbol"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// OptLevel controls the optimization aggressiveness.
type OptLevel int

const (
	O0 OptLevel = iota // No optimization (fastest compile)
	O1                 // Basic optimizations
	O2                 // Full optimization
)

// Pipeline orchestrates the compilation stages.
type Pipeline struct {
	OptLevel OptLevel
	Passes   []passes.Pass
	Ctx      *context.TranspilerContext

	// Hooks for observability (all optional)
	OnHIR func(*hir.Module) // called after HIR construction
	OnMIR func(*mir.Module) // called after MIR lowering
	OnSSA func(*ssa.Module) // called after SSA construction
}

// New creates a pipeline with the given optimization level and default passes.
// The context is empty — call NewWithContext to use pre-registered builtins.
func New(level OptLevel) *Pipeline {
	return NewWithContext(level, context.New())
}

// NewWithContext creates a pipeline with a pre-configured TranspilerContext.
func NewWithContext(level OptLevel, ctx *context.TranspilerContext) *Pipeline {
	p := &Pipeline{
		OptLevel: level,
		Ctx:      ctx,
	}

	switch level {
	case O1:
		p.Passes = []passes.Pass{
			passes.ConstFold{},
		}
	case O2:
		p.Passes = []passes.Pass{
			passes.ConstFold{},
			passes.DCE{},
		}
	}

	return p
}

// CompileTree compiles a parsed tree-sitter CST into Go source code.
// This is the full pipeline: CST → HIR → MIR → SSA → Passes → De-SSA → Backend → Codegen.
// moduleName is the Go module name for resolving relative imports.
func (p *Pipeline) CompileTree(root *sitter.Node, source []byte, pkgName, moduleName string, samePackageImports bool) ([]byte, error) {
	return p.CompileTreeWithPath(root, source, pkgName, moduleName, "", samePackageImports)
}

func (p *Pipeline) CompileTreeWithPath(root *sitter.Node, source []byte, pkgName, moduleName, sourcePath string, samePackageImports bool) ([]byte, error) {
	// Stage 1: Build HIR
	hirMod := hir.BuildModuleWithPath(root, source, pkgName, sourcePath)
	if p.OnHIR != nil {
		p.OnHIR(hirMod)
	}
	if err := hir.AsyncPipelinePhase1Error(hirMod); err != nil {
		return nil, err
	}

	// Stage 2: Lower to MIR
	mirMod := mir.Lower(hirMod)
	if p.OnMIR != nil {
		p.OnMIR(mirMod)
	}

	// Stage 3: Build SSA (skip at O0)
	if p.OptLevel > O0 && len(p.Passes) > 0 {
		ssaMod := ssa.Build(mirMod)
		if p.OnSSA != nil {
			p.OnSSA(ssaMod)
		}

		// Stage 4: Run optimization passes
		for _, pass := range p.Passes {
			if err := pass.Run(ssaMod); err != nil {
				return nil, fmt.Errorf("pass %s: %w", pass.Name(), err)
			}
		}

		// Stage 5: De-SSA back to MIR
		mirMod = ssa.DeSSA(ssaMod)
	}

	// SWC interop: if module has named exports but no default, synthesize one
	if hirMod.SynthesizeDefault == "" && !samePackageImports {
		synthesizeDefaultIfNeeded(hirMod)
	}

	// Stage 6: Backend lowering (HIR → Go AST)
	// Note: currently the backend lowers from HIR directly.
	// Once MIR→Go lowering is implemented, this will use mirMod instead.
	goFile := backend.Lower(hirMod, p.Ctx, moduleName, samePackageImports)

	// Stage 7: Codegen
	return backend.GenerateWithSource(goFile, hirMod.SourcePath, hirMod.SourceSize)
}

// CompileHIR compiles from an already-built HIR module.
func (p *Pipeline) CompileHIR(hirMod *hir.Module, moduleName string, samePackageImports bool) ([]byte, error) {
	if err := hir.AsyncPipelinePhase1Error(hirMod); err != nil {
		return nil, err
	}
	goFile := backend.Lower(hirMod, p.Ctx, moduleName, samePackageImports)
	return backend.GenerateWithSource(goFile, hirMod.SourcePath, hirMod.SourceSize)
}

// CompilePackage compiles multiple TypeScript files that belong to the same Go package.
// It scans all files for exports first, then compiles each with cross-file knowledge.
func (p *Pipeline) CompilePackage(files map[string][]byte, pkgName, moduleName, entryFile string) (map[string][]byte, error) {
	// Phase 1: Parse all files into HIR and scan exports
	hirModules := make(map[string]*hir.Module)
	allExports := make(map[string][]backend.CrossFileExport)
	allNames := make(map[string][]string)
	exportAliases := make(map[string]map[string]string)

	for name, source := range files {
		tree, err := parseTypeScript(source)
		if err != nil {
			continue
		}
		hirMod := hir.BuildModuleWithPath(tree.RootNode(), source, pkgName, name)
		tree.Close()
		if err := hir.AsyncPipelinePhase1Error(hirMod); err != nil {
			return nil, err
		}
		hirModules[name] = hirMod
		allExports[name] = backend.ScanHIRExports(hirMod)
		allNames[name] = backend.ScanHIRTopLevelNames(hirMod)
	}

	// Phase 1b: Synthesize Default export for modules with named exports
	// but no explicit default (SWC interop). Only add to the entry file.
	hasDefault := false
	for _, exps := range allExports {
		for _, exp := range exps {
			if exp.GoName == "Default" {
				hasDefault = true
			}
		}
	}
	if !hasDefault && entryFile != "" {
		if entryExps := allExports[entryFile]; entryExps != nil {
			// Find the primary named export (first exported class or function)
			for _, exp := range entryExps {
				if exp.GoName != "" && exp.GoName != "Default" {
					allExports[entryFile] = append(allExports[entryFile],
						backend.CrossFileExport{GoName: "Default", IsJSValue: true})
					// Mark the entry HIR module to synthesize var Default = PrimaryExport
					if entryMod, ok := hirModules[entryFile]; ok {
						entryMod.SynthesizeDefault = exp.GoName
					}
					break
				}
			}
		}
	}

	// Phase 2: Detect conflicting Default exports
	var defaultFiles []string
	for name, exps := range allExports {
		for _, exp := range exps {
			if exp.GoName == "Default" {
				defaultFiles = append(defaultFiles, name)
			}
		}
	}
	renameDefault := make(map[string]string) // file → renamed name
	if len(defaultFiles) > 1 {
		for _, name := range defaultFiles {
			if name != entryFile {
				renameDefault[name] = fileDefaultName(name)
			}
		}
	}

	// Update allExports for renamed files so cross-file references use the new name
	for fileName, newName := range renameDefault {
		exps := allExports[fileName]
		for i, exp := range exps {
			if exp.GoName == "Default" {
				exps[i].GoName = newName
			}
		}
	}

	// Phase 2b: assign package-global alias names for exported symbols.
	// Entry-file exports keep their public names. Non-entry exports get
	// file-specific aliases so flattened packages preserve module boundaries.
	globalUsedAliases := make(map[string]int)
	for fileName, exps := range allExports {
		aliases := make(map[string]string)
		for _, exp := range exps {
			exportName := exp.GoName
			originalName := exp.OriginalName
			if exportName == "Default" || originalName == "default" {
				alias := exportName
				if fileName != entryFile {
					alias = fileDefaultName(fileName)
				}
				aliases["default"] = makeUniqueAlias(alias, globalUsedAliases)
				continue
			}
			alias := exportName
			if fileName != entryFile {
				alias = fileSpecificExportName(fileName, originalName)
			}
			aliases[originalName] = makeUniqueAlias(alias, globalUsedAliases)
		}
		exportAliases[fileName] = aliases
	}

	// Phase 3: Compile each file with cross-file export knowledge
	results := make(map[string][]byte)
	goFiles := make(map[string]*ast.File)
	for name, hirMod := range hirModules {
		// Collect exports from OTHER files (not this one)
		var crossExports []backend.CrossFileExport
		var reservedNames []string
		importNameMap := make(map[string]string)
		for otherFile, exps := range allExports {
			if otherFile == name {
				continue
			}
			crossExports = append(crossExports, exps...)
		}
		for otherFile, names := range allNames {
			if otherFile == name {
				continue
			}
			reservedNames = append(reservedNames, names...)
			for _, alias := range exportAliases[otherFile] {
				reservedNames = append(reservedNames, alias)
			}
		}
		for _, imp := range hirMod.Imports {
			if !strings.HasPrefix(imp.ModulePath, ".") {
				continue
			}
			target := resolvePackageImportFile(name, imp.ModulePath, files)
			if target == "" {
				continue
			}
			targetAliases := exportAliases[target]
			if imp.Default != nil {
				if alias := targetAliases["default"]; alias != "" {
					importNameMap[imp.ModulePath+"\x00default"] = alias
				}
			}
			for _, n := range imp.Named {
				if alias := targetAliases[n.OriginalName]; alias != "" {
					importNameMap[imp.ModulePath+"\x00"+n.OriginalName] = alias
				}
			}
		}

		goFiles[name] = backend.LowerWithExports(hirMod, p.Ctx, moduleName, true, crossExports, reservedNames, importNameMap, exportAliases[name])
	}

	backend.BreakPackageInitCycles(goFiles)

	for name, goFile := range goFiles {
		hirMod := hirModules[name]
		out, err := backend.GenerateWithSource(goFile, hirMod.SourcePath, hirMod.SourceSize)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}

		// Rename Default in non-entry files to avoid conflicts
		if _, shouldRename := renameDefault[name]; shouldRename {
			out = renameDefaultExport(out, name)
		}

		results[name] = out
	}

	return results, nil
}

// fileDefaultName generates a file-specific name for renamed default exports.
func fileDefaultName(fileName string) string {
	base := filepath.Base(fileName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
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

func fileSpecificExportName(fileName, exportName string) string {
	clean := strings.TrimSuffix(filepath.Clean(fileName), filepath.Ext(fileName))
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) > 3 {
		parts = parts[len(parts)-3:]
	}
	name := ""
	for _, r := range strings.Join(parts, "_") + "_" + exportName {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			name += string(r)
		} else {
			name += "_"
		}
	}
	if name == "" {
		name = "File"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func makeUniqueAlias(alias string, used map[string]int) string {
	if used[alias] == 0 {
		used[alias] = 1
		return alias
	}
	used[alias]++
	return fmt.Sprintf("%s_%d", alias, used[alias])
}

func resolvePackageImportFile(currentFile, importPath string, files map[string][]byte) string {
	fromDir := filepath.Dir(currentFile)
	base := filepath.Join(fromDir, importPath)
	candidates := []string{
		base,
		base + ".ts",
		base + ".js",
		filepath.Join(base, "index.ts"),
		filepath.Join(base, "index.js"),
	}
	for _, candidate := range candidates {
		abs, _ := filepath.Abs(candidate)
		if _, ok := files[abs]; ok {
			return abs
		}
	}
	return ""
}

// renameDefaultExport replaces "Default" in compiled output with a file-specific name.
func renameDefaultExport(source []byte, fileName string) []byte {
	name := fileDefaultName(fileName)

	s := string(source)
	s = strings.ReplaceAll(s, "var Default ", "var "+name+" ")
	s = strings.ReplaceAll(s, "var Default\n", "var "+name+"\n")
	// Also rename bare assignments from fixInitCycles (e.g. "\tDefault = " in init())
	s = strings.ReplaceAll(s, "\tDefault = ", "\t"+name+" = ")
	s = strings.ReplaceAll(s, "\nDefault = ", "\n"+name+" = ")
	return []byte(s)
}

// synthesizeDefaultIfNeeded checks if a module has named exports but no default,
// and sets SynthesizeDefault to the primary named export (SWC interop).
func synthesizeDefaultIfNeeded(mod *hir.Module) {
	hasDefault := false
	var primaryExport string
	for _, d := range mod.Declarations {
		if ed, ok := d.(*hir.ExportDecl); ok {
			if ed.IsDefault {
				hasDefault = true
				return
			}
			if ed.Decl != nil {
				switch inner := ed.Decl.(type) {
				case *hir.ClassDecl:
					if inner.Symbol != nil && primaryExport == "" {
						primaryExport = symbol.Capitalize(inner.Symbol.OriginalName)
					}
				case *hir.FuncDecl:
					if inner.Symbol != nil && primaryExport == "" {
						primaryExport = symbol.Capitalize(inner.Symbol.OriginalName)
					}
				}
			}
		}
	}
	if !hasDefault && primaryExport != "" {
		mod.SynthesizeDefault = primaryExport
	}
}

// parseTypeScript is a helper to parse TypeScript source into a tree-sitter tree.
func parseTypeScript(source []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse TypeScript source")
	}
	return tree, nil
}
