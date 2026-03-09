// Package pipeline orchestrates the compiler stages from TypeScript source
// through HIR, MIR, SSA, optimization passes, and backend code generation.
//
// It provides configurable optimization levels (O0, O1, O2) and
// deterministic pass ordering. No business logic — only coordination.
package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler/backend"
	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/passes"
	"github.com/nnstd/gun/compiler/ssa"

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
	OnHIR func(*hir.Module)  // called after HIR construction
	OnMIR func(*mir.Module)  // called after MIR lowering
	OnSSA func(*ssa.Module)  // called after SSA construction
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
	// Stage 1: Build HIR
	hirMod := hir.BuildModule(root, source, pkgName)
	if p.OnHIR != nil {
		p.OnHIR(hirMod)
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

	// Stage 6: Backend lowering (HIR → Go AST)
	// Note: currently the backend lowers from HIR directly.
	// Once MIR→Go lowering is implemented, this will use mirMod instead.
	goFile := backend.Lower(hirMod, p.Ctx, moduleName, samePackageImports)

	// Stage 7: Codegen
	return backend.Generate(goFile)
}

// CompileHIR compiles from an already-built HIR module.
func (p *Pipeline) CompileHIR(hirMod *hir.Module, moduleName string, samePackageImports bool) ([]byte, error) {
	goFile := backend.Lower(hirMod, p.Ctx, moduleName, samePackageImports)
	return backend.Generate(goFile)
}

// CompilePackage compiles multiple TypeScript files that belong to the same Go package.
// It scans all files for exports first, then compiles each with cross-file knowledge.
func (p *Pipeline) CompilePackage(files map[string][]byte, pkgName, moduleName, entryFile string) (map[string][]byte, error) {
	// Phase 1: Parse all files into HIR and scan exports
	hirModules := make(map[string]*hir.Module)
	allExports := make(map[string][]backend.CrossFileExport)

	for name, source := range files {
		tree, err := parseTypeScript(source)
		if err != nil {
			continue
		}
		hirMod := hir.BuildModule(tree.RootNode(), source, pkgName)
		tree.Close()
		hirModules[name] = hirMod
		allExports[name] = backend.ScanHIRExports(hirMod)
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

	// Phase 3: Compile each file with cross-file export knowledge
	results := make(map[string][]byte)
	for name, hirMod := range hirModules {
		// Collect exports from OTHER files (not this one)
		var crossExports []backend.CrossFileExport
		for otherFile, exps := range allExports {
			if otherFile == name {
				continue
			}
			crossExports = append(crossExports, exps...)
		}

		goFile := backend.LowerWithExports(hirMod, p.Ctx, moduleName, true, crossExports)
		out, err := backend.Generate(goFile)
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
