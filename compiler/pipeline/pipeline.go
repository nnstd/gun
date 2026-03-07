// Package pipeline orchestrates the compiler stages from TypeScript source
// through HIR, MIR, SSA, optimization passes, and backend code generation.
//
// It provides configurable optimization levels (O0, O1, O2) and
// deterministic pass ordering. No business logic — only coordination.
package pipeline

import (
	"fmt"

	"github.com/nnstd/gun/compiler/backend"
	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/passes"
	"github.com/nnstd/gun/compiler/ssa"

	sitter "github.com/tree-sitter/go-tree-sitter"
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
