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
go run . transpile file.ts            # transpile a .ts file to stdout
go run . transpile file.ts -o out/    # transpile to directory (out/file.go)
go run . transpile file.ts --ast      # print tree-sitter AST
go run . build file.ts                # transpile + compile to ./file binary
go run . build file.ts -o mybin       # transpile + compile to custom path
go run . run file.ts                  # transpile, build, and execute
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
- `types.go` — TS→Go type mapping (currently used only for interfaces/structs; function params/returns all use `*jsvalue.JSValue`)
- `imports.go` — module resolution with `knownModules`/`knownSymbols` tables, relative import handling via `resolveModulePath`
- `classes.go` — class→struct with constructor function and receiver methods
- `builtin_*.go` — per-API transformers for console, Math, JSON, strings, collections
- `modules.go` — extensible `ModuleCallTransformer` registry for module-specific method call handling (e.g. Hono routes)
- `helpers.go` — Go AST builder helpers (`ident()`, `funcDecl()`, `varDecl()`, `capitalize()`, `jsvalueWrapLit()`, etc.)
- `operators.go` — binary/unary operator mapping and `jsvalueOpName()` for JSValue operation helpers

Key design patterns:
- Exported TS names get capitalized for Go visibility (`capitalize()` in helpers.go)
- `importedNames map[string]resolvedImport` tracks how each TS identifier maps to a Go package + symbol
- `varTypes map[string]string` tracks constructor origins so method calls can be dispatched to module-specific transformers
- `knownModules` maps Node.js built-in modules to Go packages; `knownSymbols` maps specific (module, symbol) pairs to exact Go translations

### Runtime

`runtime/` provides Go packages that implement Node.js-compatible APIs. **All runtime functions accept and return `*jsvalue.JSValue`:**
- `runtime/fs` — `ReadFileSync`, `WriteFileSync`, `ExistsSync`, etc. wrapping `os` package
- `runtime/path` — `Join`, `Basename`, etc. wrapping `path/filepath`
- `runtime/os` — `Homedir`, `Platform`, etc.
- `runtime/json` — `Parse`, `Stringify` for JSON handling
- `runtime/process` — `Argv`, `Env`, `Cwd`, `Exit`, `AsJSValue` for the process global
- `runtime/module` — `CreateRequire`, `ImportMeta` for module system
- `runtime/hono` — minimal Hono web framework with routing, `Context`, and `http.Handler` implementation
- `runtime/jsvalue` — `JSValue` runtime type modeling JavaScript value semantics

## All-JSValue Architecture

The transpiler uses an **all-JSValue** architecture where all variables, function parameters, and return values are `*jsvalue.JSValue`. TypeScript type annotations are ignored for code generation — they all become `*jsvalue.JSValue`.

### Core Principles

1. **All variables are `*jsvalue.JSValue`** — no native Go types for transpiled variables
2. **All function parameters are `*jsvalue.JSValue`** — TS type annotations are ignored
3. **All function return values are `*jsvalue.JSValue`** — when the function has return statements
4. **Operations unwrap temporarily** — e.g. `jsvalue.Add(a, b)` internally does `a.Number() + b.Number()`
5. **Results are re-wrapped** — operations return `*jsvalue.JSValue`
6. **Runtime functions accept `*jsvalue.JSValue`** — `fs.ReadFileSync(path *jsvalue.JSValue)`

### JSValue Operation Helpers

Binary and unary operations on JSValue use package-level helper functions instead of inline type coercion:

```go
// Arithmetic: x + y → jsvalue.Add(x, y)
// Comparison: x === y → jsvalue.Eq(x, y)
// Logical:    x || y → jsvalue.Or(x, y)
// Unary:      !x → jsvalue.Not(x)
// Typeof:     typeof x → jsvalue.TypeOf(x)
```

Available operation helpers in `runtime/jsvalue/`:
- **Arithmetic:** `Add`, `Sub`, `Mul`, `Div`, `Mod`, `Neg`, `Inc`, `Dec`
- **Bitwise:** `BitNot`, `BitAnd`, `BitOr`, `BitXor`, `Shl`, `Shr`, `UShr`
- **Comparison:** `Eq`, `NEq`, `Lt`, `Gt`, `LtE`, `GtE`
- **Logical:** `And`, `Or`, `Not`, `Nullish`
- **Type:** `TypeOf`, `IsArrayValue`

### JSValue String/Array Methods

All string and array wrapper functions accept `*jsvalue.JSValue` for ALL parameters:

```go
// All args are *JSValue:
func Replace(val, pattern, replacement *JSValue) *JSValue
func Split(val, sep *JSValue) *JSValue
func Join(arr, sep *JSValue) *JSValue
func Slice(arr *JSValue, args ...*JSValue) *JSValue
func CharAt(val, index *JSValue) *JSValue
func Substring(str, start *JSValue, end ...*JSValue) *JSValue

// Collection methods:
func Map(arr *JSValue, fn any) *JSValue
func Filter(arr *JSValue, fn any) *JSValue
func ForEach(arr *JSValue, fn any)
func Find(arr *JSValue, fn any) *JSValue
func Some(arr *JSValue, fn any) *JSValue
func Every(arr *JSValue, fn any) *JSValue
func Reduce(arr *JSValue, fn any, initial ...*JSValue) *JSValue
```

### Boolean Contexts

In Go `if`/`for` conditions, JSValue expressions use `.Bool()` for truthiness:

```go
// JSValue in boolean context → .Bool()
if jsvalue.Not(flag).Bool() { ... }
if jsvalue.Lt(a, b).Bool() { ... }
if jsvalue.And(x, y).Bool() { ... }
```

The `ensureBool()` function in the compiler handles this conversion. It calls `.Bool()` for JSValue expressions and passes through native Go booleans unchanged.

### Wrapping Literal Arguments

When calling jsvalue functions or runtime package functions, literal arguments are wrapped using `jsvalueWrapLit()`:

```go
// String literal → jsvalue.NewString("...")
// Int literal → jsvalue.NewNumber(float64(...))
// Bool literal → jsvalue.NewBool(...)
// Negative literal → jsvalue.NewNumber(float64(-N))
// Unknown expression → jsvalue.From(expr)
```

### Property Access on JSValue

- **Local JSValue variables:** `obj.foo` → `obj.Get("foo")`
- **Package-level untyped vars:** `obj.foo` → `obj.Get("foo")`
- **Cross-file package vars:** `Enum.VALUE` → `Enum.Get("VALUE")`
- **Typed struct locals:** `parser.parse(x)` → `parser.Parse(x)` (capitalized)

### Key Tracking Maps in Transformer

- `jsvalueLocals` — local variables holding `*jsvalue.JSValue`
- `jsvalueSliceLocals` — typed locals holding `[]*jsvalue.JSValue` (rest params, array literals)
- `pkgVarTyped` — package-level variables: `true` = typed struct, `false` = JSValue
- `localScopes` — scope stack tracking whether variables are typed or JSValue
- `funcParamCounts` — hoisted function parameter counts for nil-padding

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

Regression tests for the all-JSValue refactoring are in `compiler/regression_test.go`.

Use `compileWithModule(t, ts, "myapp")` when testing relative import resolution. Helpers are in `compiler/helpers_test.go`.

## Testing Transpilation with Real Projects

### gun-test Project

The `gun-test` project at `~/Work/gun-test/` is a test project with real npm dependencies (yargs, string-width, etc.) that can be used to verify transpilation works correctly with complex modules.

**Transpile the project:**
```bash
go run . transpile ~/Work/gun-test/index.ts -o /tmp/gun-out/
```

**Verify transpilation:**
```bash
ls /tmp/gun-out/  # Should show transpiled modules: string_width, yargs_parser, etc.
```

**Compile the transpiled output:**
```bash
cd /tmp/gun-out && go build .
```

**Clean up before retranspiling:**
```bash
rm -rf /tmp/gun-out
```

This workflow is useful for testing compiler changes that affect how imported modules are transpiled, especially for verifying type coercion, import handling, and built-in transformations work correctly with real-world code.
