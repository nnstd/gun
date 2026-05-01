package backend

import "github.com/nnstd/gun/compiler/symbol"

// CPUProfileConfig controls optional generated-main CPU profiling support.
// Empty Dir/Name values mean "use runtime defaults".
type CPUProfileConfig struct {
	Dir  string
	Name string
}

// LowerConfig holds all optional configuration for the HIR→Go AST lowering pass.
type LowerConfig struct {
	CrossFileExports []CrossFileExport
	ReservedNames    []string
	ImportNameMap    map[string]string
	ExportAliasMap   map[string]string
	LocalAliasMap    map[symbol.ID]string
	NamespaceAlias   string
	NamespaceEntries map[string]string
	CPUProfile       *CPUProfileConfig
	Otel             bool
	IsEntryFile      bool
}
