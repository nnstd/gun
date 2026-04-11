# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gun

Gun is a TypeScript-to-Go transpiler. It parses TypeScript using tree-sitter, transforms the CST through a multi-stage compiler pipeline (HIR → MIR → SSA → Go AST), and emits formatted Go source via `go/format`.

## Commands

```bash
go build ./...              # build everything
go test ./compiler/         # run compiler tests (production path)
go test ./compiler/ -run TestExportDefaultClass   # run a single test
go test ./...               # run all tests including runtime and new pipeline
go run . transpile file.ts            # transpile a .ts file to stdout
go run . transpile file.ts -o out/    # transpile to directory (out/file.go)
go run . transpile file.ts --ast      # print tree-sitter AST
go run . build file.ts                # transpile + compile to ./file binary
go run . build file.ts -o mybin       # transpile + compile to custom path
go run . run file.ts                  # transpile, build, and execute
```

## Architecture

The project has two compilation paths. The **default path** goes through the new pipeline (HIR → MIR → SSA → Backend). The original `Transformer` remains in the tree as a legacy implementation/reference path. Both share the same symbol table and TranspilerContext foundations.

### Production Path

Three phases orchestrated by `compiler.Compile()` in `compiler/compiler.go`:

1. **Parse** (`compiler/parser.go`) — tree-sitter parses TypeScript into a CST
2. **Transform** (`compiler/transformer.go`) — walks the CST and builds `go/ast.File` using `TranspilerContext` for builtin dispatch
3. **Emit** (`compiler/emitter.go`) — `go/format.Node()` pretty-prints the Go AST

### New Multi-Stage Pipeline

Full pipeline orchestrated by `compiler/pipeline/`:

```
TypeScript source
       ↓  tree-sitter
    CST
       ↓  compiler/hir/builder.go
    HIR  (preserves TS semantics, symbol-based identifiers)
       ↓  compiler/mir/lower.go
    MIR  (basic blocks, explicit CFG, desugared control flow)
       ↓  compiler/ssa/build.go
    SSA  (single assignment, phi nodes, dominator tree)
       ↓  compiler/passes/
    Optimized SSA  (constant folding, dead code elimination)
       ↓  compiler/ssa/dessa.go
    MIR
       ↓  compiler/backend/lower.go
    Go AST  (JSValue-wrapped Go code)
       ↓  compiler/backend/codegen.go
    Go source text
```

### Package dependency direction (never reversed)

```
symbol, context    (foundation — no deps on compiler stages)
       ↓
      hir          (depends on symbol)
       ↓
      mir          (depends on hir, symbol)
       ↓
      ssa          (depends on mir, symbol)
       ↓
    passes         (depends on ssa)
       ↓
    backend        (depends on hir, symbol, context)
       ↓
    pipeline       (depends on all above — orchestration only)
```

## Compiler Packages

### `compiler/symbol/` — Hygienic symbol table

Replaces string-based identifier tracking with unique symbol IDs. Every identifier gets a `Symbol` with a unique numeric `ID`. Go names are generated only at emission time via `EmitName()`, guaranteeing collision-free output.

- `symbol.go` — `Symbol` (ID, OriginalName, Kind, Exported, IsJSValue, etc.), `Sanitize()`, `Capitalize()`
- `table.go` — `Table` with `PushScope`/`PopScope`/`Define`/`Lookup`/`EmitName`/`ReserveName`

**Where to make changes:** Add new symbol kinds here. If a new identifier tracking concept is needed, add fields to `Symbol` — don't add new maps to the Transformer.

### `compiler/context/` — TranspilerContext (builtin registry)

Unified registry for all JS global objects, functions, constructors, identifiers, and modules. Replaces hardcoded switch/case dispatch.

- `context.go` — `TranspilerContext` with `RegisterGlobal`, `RegisterGlobalFunc`, `RegisterConstructor`, `RegisterIdentifier`, `RegisterModule`, `MarkKnownGlobal`, and lookup/dispatch methods

**Where to make changes:** To add a new JS builtin (global object, function, constructor), register it in `compiler/context_defaults.go` — NOT in `builtins.go`. The context is the single source of truth for what builtins exist.

### `compiler/context_defaults.go` — Builtin registrations

Lives in the `compiler` package (not `context/`) because the registration closures use compiler-package helpers like `callExpr`, `selectorExpr`, `jsvalueWrapLit`.

- `registerGlobalObjects()` — console, Math, JSON, Object, Number, process, Array, Error types
- `registerGlobalFunctions()` — isNaN, isFinite, Number, Array, Symbol, String, parseInt, parseFloat, Error types
- `registerConstructors()` — Error types, Map, WeakMap, Set, WeakSet, Array, Date, RegExp, Hono, IntlSegmenter
- `registerIdentifierMappings()` — undefined, null, Infinity, NaN, console, Math, JSON, Error types, Object, process, Array, String, Promise, Number, Boolean
- `registerModules()` — fs, path, os, hono, http, url, util, events, stream, buffer, crypto, child_process, assert, module
- `registerKnownGlobals()` — marks names that should NOT get `.Get()` dispatch (typed globals vs JSValue wrappers)

**Where to make changes:** All new builtins go here. To add a new global object like `Intl`, add a `RegisterGlobal` call with its methods. To add a new module like `net`, add a `RegisterModule` call. To add a new constructor like `WeakRef`, add a `RegisterConstructor` call.

### `compiler/hir/` — High-Level Intermediate Representation

Preserves TypeScript semantics without Go-specific constructs. All identifiers reference `*symbol.Symbol`.

- `nodes.go` — Module, declarations (FuncDecl, VarDecl, ClassDecl, EnumDecl, InterfaceDecl, TypeAliasDecl, ImportDecl, ExportDecl), statements (15 types), expressions (30+ types), patterns (ObjectPattern, ArrayPattern)
- `builder.go`, `builder_stmt.go`, `builder_expr.go` — walks tree-sitter CST, produces `*hir.Module`
- `printer.go` — human-readable debug output

**Where to make changes:** To support a new TS syntax construct (e.g. decorators), add an HIR node type in `nodes.go` and handle it in the builder. HIR should represent the TS semantics faithfully — no Go-specific lowering.

### `compiler/mir/` — Mid-Level Intermediate Representation

Normalizes HIR into basic blocks with explicit control flow graph (CFG). Desugars JS-only constructs.

- `mir.go` — Module, Function, BasicBlock, Terminator types (Jump, Branch, Return, Switch, Panic), flat statements (Assign, Store, Expr, Decl, Defer), pure expressions
- `lower.go` — HIR → MIR lowering, builds CFG with proper predecessor/successor edges, lowers if/while/for/switch/try-catch into block structure
- `printer.go` — shows basic blocks with terminators and CFG edges

**Where to make changes:** Desugaring logic goes here. Optional chaining, nullish coalescing, destructuring, and any JS→Go semantic translation that doesn't need Go AST specifics. If adding a new control flow pattern, define the block structure and terminator here.

### `compiler/ssa/` — Static Single Assignment

Converts MIR to SSA form for optimization.

- `ssa.go` — SSAValue, Const, Phi, instructions (BinInstr, UnaryInstr, CallInstr, GetInstr, SetInstr, NewInstr, AllocInstr, CopyInstr), terminators
- `domtree.go` — `ComputeDominators()` (Cooper-Harvey-Kennedy), `ComputeDominanceFrontiers()`
- `build.go` — MIR → SSA: creates SSA blocks, wires CFG, converts statements to instructions, inserts phi nodes via iterated dominance frontier
- `dessa.go` — SSA → MIR: eliminates phi nodes by inserting copies at predecessor block ends

**Where to make changes:** New instruction types go in `ssa.go`. The SSA builder in `build.go` handles converting MIR statements/expressions to SSA instructions.

### `compiler/passes/` — Optimization passes

Composable passes operating on SSA form.

- `pass.go` — `Pass` interface: `Name() string`, `Run(mod *ssa.Module) error`
- `constfold.go` — constant folding (evaluates `number op number` and `string + string` at compile time)
- `dce.go` — dead code elimination (removes instructions with unused results, preserves side effects)

**Where to make changes:** Add new optimization passes here. Each pass implements the `Pass` interface. Passes must not depend on the backend or emit Go code.

### `compiler/pipeline/` — Pass manager and orchestration

Orchestrates the full compilation pipeline with configurable optimization levels.

- `pipeline.go` — `Pipeline` with `OptLevel` (O0/O1/O2), `CompileTree()` (full pipeline), `CompileHIR()` (direct), observability hooks (`OnHIR`, `OnMIR`, `OnSSA`)

O0 = no optimization (skip SSA entirely), O1 = constant folding, O2 = constant folding + DCE.

**Where to make changes:** To add a new optimization level or change pass ordering, modify `New()`. To add a new pipeline stage, modify `CompileTree()`.

### `compiler/backend/` — Go lowering and code generation

Converts HIR to `go/ast` and formats as Go source.

- `lower.go` — `Lower(mod *hir.Module, ctx *context.TranspilerContext) *ast.File` — lowers declarations, functions (main/init as Go funcs, others as jsvalue.NewFunction), classes, enums, interfaces
- `lower_stmt.go` — lowers statements: if, for, for-in (→ range OwnKeys), for-of (→ range Array), while, do-while, switch, try/catch (→ defer/recover), throw (→ panic)
- `lower_expr.go` — lowers expressions: literals → JSValue constructors, binary/unary ops → jsvalue helpers, member access → `.Get()`, calls, ternary → IIFE, arrow functions → jsvalue.NewFunction
- `codegen.go` — `Generate(file *ast.File) ([]byte, error)` — formats via `go/format.Node()`
- `builders.go` — Go AST construction helpers (goIdent, callExpr, selectorExpr, jsvalueWrapLit, etc.)
- `operators.go` — HIR operator → jsvalue helper name mapping

**Where to make changes:** Go-specific code generation goes here. JSValue wrapping, Go AST construction, import assembly. If adding a new Go output pattern (e.g. goroutine generation for async), add it here.

### Production Transformer (`compiler/` root)

The existing production transpiler. Uses `TranspilerContext` for builtin dispatch but still does direct CST → Go AST transformation.

- `transformer.go` — main `Transformer` struct, `transform()`, scope management, `AddImport`/`AddAliasedImport` (implements `context.Imports`)
- `declarations.go`, `statements.go`, `expressions.go`, `classes.go` — CST → Go AST transformation by concern
- `imports.go` — module resolution, `resolveIdentifier()` delegates to `TranspilerContext`
- `builtins.go` — `isErrorType()` helper; old dispatch functions removed (now in context_defaults.go)
- `builtin_*.go` — per-API transform implementations (console, math, json, collections, regexp) — called by context registrations
- `helpers.go` — Go AST builder helpers shared by transformer and context registrations
- `operators.go` — operator mapping (legacy, also in backend/operators.go)
- `modules.go` — `ModuleCallTransformer` registry for module-specific dispatch (Hono, etc.)

**Where to make changes:** Bug fixes to the production transpiler go in these files. New TS syntax support that must work immediately goes here. For new features that can wait, implement in the new pipeline (hir → backend) instead.

### Runtime

`runtime/` provides Go packages that implement Node.js-compatible APIs. **All runtime functions accept and return `*jsvalue.JSValue`:**
- `runtime/builtin` — `JSValue` type, array/string/number/object/map/set methods, prototype chains, property descriptors, regex
- `runtime/builtin/console` — `Log`, `Error`, `Warn`, `Dir`
- `runtime/builtin/math` — `Floor`, `Ceil`, `Round`, `Abs`, `Max`, `Min`, `Sqrt`, `Pow`, `Random`, etc.
- `runtime/builtin/json` — `Parse`, `Stringify`
- `runtime/builtin/error` — `Error`, `TypeError`, `RangeError`, etc. constructors
- `runtime/builtin/intl` — `Segmenter`
- `runtime/fs` — `ReadFileSync`, `WriteFileSync`, `ExistsSync`, etc.
- `runtime/path` — `Join`, `Basename`, etc.
- `runtime/os` — `Homedir`, `Platform`, etc.
- `runtime/process` — `Argv`, `Env`, `Cwd`, `Exit`, `AsJSValue`
- `runtime/module` — `CreateRequire`, `ImportMeta`
- `runtime/hono` — minimal Hono web framework

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

These maps remain in the production Transformer and will gradually migrate to the symbol table:

- `jsvalueLocals` — local variables holding `*jsvalue.JSValue`
- `jsvalueSliceLocals` — typed locals holding `[]*jsvalue.JSValue` (rest params, array literals)
- `pkgVarTyped` — package-level variables: `true` = typed struct, `false` = JSValue
- `localScopes` — scope stack tracking whether variables are typed or JSValue
- `funcParamCounts` — hoisted function parameter counts for nil-padding
- `ctx` — `*context.TranspilerContext` for builtin dispatch (replaces old switch/case)

## Test conventions

### Production compiler tests

All production compiler tests live in `compiler/*_test.go`:

```go
func TestFeatureName(t *testing.T) {
    ts := `TypeScript source here`
    out := compile(t, ts)                    // compiles as package "main"
    assertContains(t, out, "expected Go")
    assertNotContains(t, out, "unwanted")
}
```

Use `compileWithModule(t, ts, "myapp")` when testing relative import resolution. Helpers are in `compiler/helpers_test.go`.

### New pipeline tests

Each new package has its own tests:

- `compiler/symbol/table_test.go` — scope push/pop, name collision, sanitization
- `compiler/context/context_test.go` — registration and dispatch
- `compiler/hir/builder_test.go` — TS → HIR for all node types, printer output
- `compiler/mir/lower_test.go` — HIR → MIR, CFG structure, block counts, edge verification
- `compiler/ssa/build_test.go` — MIR → SSA, dominator tree, de-SSA round-trip
- `compiler/passes/passes_test.go` — constant folding, dead code elimination
- `compiler/backend/lower_test.go` — HIR → Go AST (unit tests and round-trip TS → Go)
- `compiler/pipeline/pipeline_test.go` — full pipeline at O0/O1/O2, hooks, various TS snippets

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
