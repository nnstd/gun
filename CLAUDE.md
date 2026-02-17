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

## JSValue Usage Patterns

The `runtime/jsvalue` package provides the `JSValue` type that models JavaScript value semantics in Go. Understanding when and how to use JSValue is critical for correct transpilation.

### When to Use JSValue vs Native Go Types

**Use JSValue (`*jsvalue.JSValue`) for:**
- Untyped function parameters (no TypeScript type annotation)
- Destructured variables from objects/arrays
- Variables that may hold different types at runtime
- Return values from functions without explicit return type annotations
- Default values in destructuring patterns

**Use native Go types for:**
- Variables with explicit TypeScript type annotations (`: number`, `: string`, `: boolean`)
- Function parameters with type annotations
- Function return types with explicit annotations
- Literals in regular variable declarations

### Boolean Literal Handling

Boolean literals have context-dependent behavior:

**Regular variable declarations** → Plain Go booleans:
```typescript
const flag = false;
if (!flag) { return true; }
```
```go
var flag = false
if !flag {
    return true
}
```

**Destructuring with defaults** → JSValue:
```typescript
const { enabled = true } = options;
if (!enabled) { return "disabled"; }
```
```go
enabled := jsvalue.NewBool(true)
if !enabled.Bool() {
    return "disabled"
}
```

**Key principle:** Destructured fields are always JSValue because they come from objects that may have undefined properties.

### Type Coercion Patterns

**Augmented assignment with numeric types:**
```typescript
let width: number = 0;
width += someJSValue;
```
```go
var width float64 = 0
width += someJSValue.Number()
```

**Augmented assignment with string types:**
```typescript
let result = "";
result += someJSValue;
```
```go
var result = ""
result += fmt.Sprint(someJSValue)
```

**Negation operator on JSValue:**
```typescript
if (!jsValue) { ... }
```
```go
if !jsValue.Bool() { ... }
```

**Comparison operators:**
```typescript
if (jsValue === false) { ... }
```
```go
if jsValue.Bool() == false { ... }
```

### Destructuring and JSValue

All destructured variables are tracked as JSValue, regardless of their default values:

```typescript
function f(options) {
    const { name, age = 0, enabled = false } = options;
    // name, age, and enabled are all JSValue
}
```

```go
func f(options *jsvalue.JSValue) *jsvalue.JSValue {
    var name = options.Name
    var age = jsvalue.NewNumber(0)
    var enabled = jsvalue.NewBool(false)
    // All three are *jsvalue.JSValue
}
```

**Why?** Because `options` might not have these properties, so the variables must be able to hold the default value OR the property value, both as JSValue.

### Function Return Types

**Explicit return type annotation** → Use the annotated type:
```typescript
function isOk(x: number): boolean { return x > 0; }
```
```go
func isOk(x float64) bool {
    return x > 0
}
```

**No return type annotation** → Default to JSValue:
```typescript
function process(x) { return x; }
```
```go
func process(x *jsvalue.JSValue) *jsvalue.JSValue {
    return x
}
```

The compiler tracks function return types in `funcReturnTypes map[string]string` so that `ensureBool()` can avoid adding `!= nil` checks to functions that return `bool`.

### Truthiness Semantics

JavaScript truthiness is implemented via the `.Bool()` method on JSValue:

**Falsy values in JavaScript:**
- `false`, `0`, `""`, `null`, `undefined`, `NaN`

**Truthy values:**
- Everything else, including `[]`, `{}`, `"0"`, `"false"`

**Implementation:**
```go
if !jsValue.Bool() {
    // Handles all falsy cases correctly
}
```

**Never use nil checks for truthiness:**
```go
// ❌ Wrong - only checks if pointer is nil
if jsValue == nil { ... }

// ✅ Correct - checks JavaScript truthiness
if !jsValue.Bool() { ... }
```

### Common Pitfalls

1. **Comparing JSValue to nil for truthiness**
   - `jsValue == nil` only checks if the pointer is nil
   - Use `!jsValue.Bool()` for JavaScript truthiness

2. **Forgetting to wrap default values in destructuring**
   - Default values must be wrapped: `jsvalue.NewBool(false)`, not `false`
   - The `wrapInJSValue()` helper handles this automatically

3. **Adding `!= nil` to boolean function calls**
   - Functions with `: boolean` return type return `bool`, not `*jsvalue.JSValue`
   - Check `funcReturnTypes` before adding nil checks

4. **Using wrong coercion for augmented assignment**
   - Numeric types need `.Number()`, not `fmt.Sprint()`
   - String types need `fmt.Sprint()`, not `.String()` (which may not exist)

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
