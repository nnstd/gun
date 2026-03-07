// Package passes provides composable optimization and analysis passes
// that operate on SSA form. Each pass implements the Pass interface.
package passes

import "github.com/nnstd/gun/compiler/ssa"

// Pass is the interface for all optimization and analysis passes.
type Pass interface {
	Name() string
	Run(mod *ssa.Module) error
}
