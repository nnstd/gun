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
- `runtime/jsvalue` — `JSValue` runtime type modeling JavaScript value semantics: typed factories, prototype chain, property descriptors

## Transpiler Practices and Conventions

### Wrapper Function Approach

When implementing transformations for JavaScript/TypeScript methods, prefer **runtime wrapper functions** over inline code generation:

**❌ Avoid:** Generating verbose inline IIFEs or complex expressions in the compiler
```go
// Bad: 30+ lines of IIFE generation in compiler
return &ast.CallExpr{
    Fun: &ast.FuncLit{
        Type: &ast.FuncType{...},
        Body: &ast.BlockStmt{
            List: []ast.Stmt{arrDecl, ifStmt, returnUndef},
        },
    },
}
```

**✅ Prefer:** Simple wrapper function calls
```go
// Good: 1 line in compiler, complexity in runtime
return callExpr(selectorExpr(ident("jsvalue"), "Pop"), obj)
```

### Wrapper Function Design Rules

When adding wrapper functions to `runtime/jsvalue/`:

1. **Return Type Consistency:** Always return `*jsvalue.JSValue`, never primitive types
   ```go
   // ✅ Correct
   func Includes(arr *JSValue, val *JSValue) *JSValue {
       return NewBool(result)
   }

   // ❌ Wrong - causes type mismatches in transpiled code
   func Includes(arr *JSValue, val *JSValue) bool {
       return result
   }
   ```

2. **Nil Safety:** All wrapper functions must handle nil inputs gracefully
   ```go
   func Pop(arr *JSValue) *JSValue {
       if arr == nil || arr.arrayVal == nil {
           return NewUndefined()
       }
       // ... implementation
   }
   ```

3. **JavaScript Semantics:** Maintain JS behavior (undefined for empty arrays, negative indices, truthiness, etc.)
   ```go
   func Slice(arr *JSValue, args ...int) *JSValue {
       // Handle negative indices like JavaScript
       if idx < 0 {
           idx = length + idx
       }
       // ...
   }
   ```

4. **Package-Level Functions:** Implement as package-level functions, not methods on `*JSValue`
   ```go
   // ✅ Correct
   func Pop(arr *JSValue) *JSValue

   // ❌ Wrong - harder to call from compiler
   func (v *JSValue) Pop() *JSValue
   ```

### Compiler Transformation Rules

When updating compiler transformations to use wrapper functions:

1. **Wrap Arguments:** Always wrap literal arguments with `jsvalue.From()` when calling wrapper functions
   ```go
   // ✅ Correct
   wrappedArg := callExpr(selectorExpr(ident("jsvalue"), "From"), args[0])
   return callExpr(selectorExpr(ident("jsvalue"), "Includes"), obj, wrappedArg)

   // ❌ Wrong - causes "cannot use 42 as *jsvalue.JSValue" errors
   return callExpr(selectorExpr(ident("jsvalue"), "Includes"), obj, args[0])
   ```

2. **Import Management:** Add the jsvalue import when using wrapper functions
   ```go
   addImport("github.com/nnstd/gun/runtime/jsvalue")
   ```

3. **Fallback for Typed Arrays:** Keep native Go transformations for typed arrays
   ```go
   if isJSValueReceiver || isJSValueMethodCall(obj) {
       // Use jsvalue wrapper
       return callExpr(selectorExpr(ident("jsvalue"), "Pop"), obj)
   }
   // For typed arrays: arr[len(arr)-1]
   return &ast.IndexExpr{X: obj, Index: lastIndex}
   ```

### When to Add Wrapper Functions

Add wrapper functions when:
- The transformation generates 10+ lines of code
- The transformation uses IIFEs for control flow
- The same logic is needed in multiple places
- The operation has complex JavaScript semantics (negative indices, truthiness, etc.)

Keep inline transformations when:
- The transformation is 1-2 lines
- It's a direct mapping to Go stdlib (e.g., `Math.floor` → `math.Floor`)
- No special JavaScript semantics are needed

### Testing Wrapper Functions

1. **Runtime tests** in `runtime/jsvalue/jsvalue_test.go`:
   - Test nil safety
   - Test edge cases (empty arrays, negative indices, etc.)
   - Test JavaScript semantics

2. **Compiler tests** in `compiler/builtins_test.go`:
   - Verify wrapper function calls are generated
   - Check that arguments are properly wrapped
   - Ensure no IIFEs are generated

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
