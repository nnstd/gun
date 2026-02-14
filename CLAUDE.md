# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gun

Gun is a TypeScript-to-Go transpiler. It parses TypeScript using tree-sitter, transforms the CST into a Go AST, and emits formatted Go source via `go/format`.

## Commands

```bash
go build ./...              # build everything
go test ./compiler/         # run compiler tests
go test ./compiler/ -run TestExportDefaultClass   # run a single test
go test ./...               # run all tests including runtime
go run . build file.ts      # transpile a .ts file to stdout
go run . build file.ts -o out.go   # transpile to file
go run . run file.ts        # transpile, build, and execute
```

## Architecture

The compiler pipeline is three phases, all orchestrated by `compiler.Compile()` in `compiler/compiler.go`:

1. **Parse** (`compiler/parser.go`) — tree-sitter parses TypeScript into a concrete syntax tree
2. **Transform** (`compiler/transformer.go`) — walks the CST and builds a `go/ast.File`. The `Transformer` struct accumulates declarations, tracks imports, and maps TS names to Go resolutions
3. **Emit** (`compiler/emitter.go`) — `go/format.Node()` pretty-prints the Go AST

### Transformer internals

The transformer is split across multiple files by concern:

- `declarations.go` — functions, variables, interfaces, enums, type aliases, destructuring
- `statements.go` — control flow, loops, try/catch→defer/recover, assignments
- `expressions.go` — literals, operators, calls, arrow functions, template strings, object literals
- `types.go` — TS→Go type mapping (e.g. `number`→`float64`, `T | null`→`*T`)
- `imports.go` — module resolution with `knownModules`/`knownSymbols` tables, relative import handling via `resolveModulePath`
- `classes.go` — class→struct with constructor function and receiver methods
- `builtin_*.go` — per-API transformers for console, Math, JSON, strings, collections
- `modules.go` — extensible `ModuleCallTransformer` registry for module-specific method call handling (e.g. Hono routes)
- `helpers.go` — Go AST builder helpers (`ident()`, `funcDecl()`, `varDecl()`, `capitalize()`, etc.)

Key design patterns:
- Exported TS names get capitalized for Go visibility (`capitalize()` in helpers.go)
- `importedNames map[string]resolvedImport` tracks how each TS identifier maps to a Go package + symbol
- `varTypes map[string]string` tracks constructor origins so method calls can be dispatched to module-specific transformers
- `knownModules` maps Node.js built-in modules to Go packages; `knownSymbols` maps specific (module, symbol) pairs to exact Go translations

### Runtime

`runtime/` provides Go packages that implement Node.js-compatible APIs:
- `runtime/fs` — `ReadFileSync`, `WriteFileSync`, `ExistsSync`, etc. wrapping `os` package
- `runtime/path` — `Join`, `Basename`, etc. wrapping `path/filepath`
- `runtime/os` — `Homedir`, `Platform`, etc.
- `runtime/hono` — minimal Hono web framework with routing, `Context`, and `http.Handler` implementation

## Test conventions

All compiler tests live in `compiler/*_test.go` and follow this pattern:

```go
func TestFeatureName(t *testing.T) {
    ts := `TypeScript source here`
    out := compile(t, ts)                    // compiles as package "main"
    assertContains(t, out, "expected Go")
    assertNotContains(t, out, "unwanted")
}
```

Use `compileWithModule(t, ts, "myapp")` when testing relative import resolution. Helpers are in `compiler/helpers_test.go`.
