---
title: C Compiler
lead: Compile and call C code inline from TypeScript. No separate build step.
sections:
  - Compile C from TypeScript
  - Define symbols
  - Link system libraries
  - N-API interop
---

## Compile C from TypeScript

Use `cc` from `bun:ffi` to compile C source and call it immediately:

```ts
import { cc } from "bun:ffi";

const { symbols } = cc({
  source: "./math.c",
  symbols: {
    add: { args: ["int", "int"], returns: "int" },
  },
});

console.log(symbols.add(3, 4)); // 7
```

```c
// math.c
int add(int a, int b) {
  return a + b;
}
```

No `Makefile`, no `cargo`, no `node-gyp`. The C code is compiled on first use and linked into the running process.

## Define symbols

The `symbols` object maps C function names to their FFI signatures:

```ts
const { symbols } = cc({
  source: "./image.c",
  symbols: {
    grayscale: {
      args: ["ptr", "int", "int"],
      returns: "int",
    },
    resize: {
      args: ["ptr", "int", "int", "ptr", "int", "int"],
      returns: "int",
    },
  },
});
```

Each entry specifies argument types and the return type using the same FFI type system (`int`, `ptr`, `cstring`, etc.).

## Link system libraries

Pass `library` to link against installed shared libraries:

```ts
const { symbols } = cc({
  source: "./sqlite_query.c",
  symbols: {
    query_version: { args: [], returns: "cstring" },
  },
  library: ["sqlite3"],
  define: { NDEBUG: "1" },
});
```

- `library` — link flags (e.g. `["sqlite3"]` links `-lsqlite3`)
- `define` — preprocessor defines passed to the compiler
- `flags` — additional compiler flags (include paths, warnings)

## N-API interop

For functions that need to return JavaScript values (strings, objects, arrays), use N-API:

```ts
const { symbols } = cc({
  source: "./greeting.c",
  symbols: {
    greet: { args: ["napi_env"], returns: "napi_value" },
  },
});

console.log(symbols.greet()); // "Hello from C!"
```

```c
#include <node/node_api.h>

napi_value greet(napi_env env) {
  napi_value result;
  napi_create_string_utf8(env, "Hello from C!", NAPI_AUTO_LENGTH, &result);
  return result;
}
```

N-API allows full interop with JavaScript object types without manual serialization.
