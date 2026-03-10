# Gun

A TypeScript-to-Go transpiler.

## Install

```bash
go install github.com/nnstd/gun@latest
```

Or build from source:

```bash
git clone https://github.com/nnstd/gun.git
cd gun
go build .
```

## Usage

### Transpile

Convert TypeScript to Go source:

```bash
gun transpile file.ts              # print Go source to stdout
gun transpile file.ts -o out/      # write to output directory
gun transpile src/ -o out/         # transpile a directory
```

### Build

Transpile and compile to a native binary:

```bash
gun build file.ts                  # produces ./file binary
gun build file.ts -o mybin         # custom output path
```

### Run

Transpile, compile, and execute in one step:

```bash
gun run file.ts
gun run file.ts -- --flag arg      # pass arguments to the program
```

### Options

| Flag | Description |
|------|-------------|
| `-o, --output` | Output directory (transpile) or binary path (build) |
| `-p, --pkg` | Go package name (default: `main`) |
| `-v, --verbose` | Verbose output |
| `--ast` | Print the tree-sitter AST instead of transpiling |

## Example

Input (`hello.ts`):

```typescript
const greet = (name: string): string => {
  return `Hello, ${name}!`;
};

console.log(greet("world"));
```

Transpile and run:

```bash
gun run hello.ts
# Hello, world!
```

## How It Works

Gun uses an **all-JSValue architecture** — all transpiled variables, parameters, and return values are `*jsvalue.JSValue`. TypeScript type annotations are used for parsing but ignored during code generation. Operations unwrap values temporarily (e.g., `jsvalue.Add(a, b)`) and re-wrap the results.

### Compilation Pipeline

```
TypeScript source
       ↓  tree-sitter
     CST
       ↓  transformer
   Go AST (go/ast)
       ↓  go/format
   Go source
```

### Runtime

Transpiled code depends on Gun's runtime packages, which provide Node.js-compatible APIs:

- `runtime/builtin` — JSValue type system, array/string/number/object methods
- `runtime/builtin/console` — `console.log`, `console.error`, etc.
- `runtime/builtin/math` — `Math.floor`, `Math.random`, etc.
- `runtime/builtin/json` — `JSON.parse`, `JSON.stringify`
- `runtime/fs` — `fs.readFileSync`, `fs.writeFileSync`, etc.
- `runtime/path` — `path.join`, `path.basename`, etc.
- `runtime/os` — `os.homedir`, `os.platform`, etc.
- `runtime/process` — `process.argv`, `process.env`, `process.exit`

## Supported Features

- Variables and constants (`let`, `const`, `var`)
- Functions (declarations, expressions, arrow functions, default parameters, rest parameters)
- Classes (constructors, methods, inheritance, static members)
- Control flow (`if`/`else`, `for`, `for...of`, `for...in`, `while`, `do...while`, `switch`)
- Error handling (`try`/`catch`/`finally`, `throw`)
- Template literals
- Destructuring (object and array patterns)
- Spread operator
- Enums
- Module system (`import`/`export`)
- Built-in globals (`console`, `Math`, `JSON`, `Object`, `Array`, `Error`, `process`)
- Node.js modules (`fs`, `path`, `os`, `url`, `crypto`, `assert`, and more)

## Development

```bash
go build ./...          # build everything
go test ./compiler/     # run compiler tests
go test ./...           # run all tests
```

## License

See [LICENSE](LICENSE) for details.
