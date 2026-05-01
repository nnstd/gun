package compiler

import "github.com/nnstd/gun/compiler/backend"

// CPUProfileConfig enables generated-main CPU profiling for gun run.
// Empty Dir/Name values use runtime defaults.
type CPUProfileConfig = backend.CPUProfileConfig

// CompileOptions carries optional codegen-only compile toggles.
type CompileOptions struct {
	CPUProfile *CPUProfileConfig
	Otel       bool
}
