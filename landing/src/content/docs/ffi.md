---
title: FFI
lead: Call native C libraries from TypeScript with zero build steps using Gun's built-in FFI.
sections:
  - Load a native library
  - Supported types
  - Pointers and buffers
  - Callbacks
  - Platform notes
---

## Load a native library

Use `dlopen` to load a shared library and call its functions directly:

```ts
import { dlopen, suffix } from "bun:ffi";

const lib = dlopen(`libsqlite3.${suffix}`, {
  sqlite3_libversion: {
    args: [],
    returns: "cstring",
  },
});

console.log(`SQLite version: ${lib.symbols.sqlite3_libversion()}`);
lib.close();
```

No separate build step or native binding compilation required.

## Supported types

FFI arguments and return values support these types:

| Type       | Description                          |
|------------|--------------------------------------|
| `i8`–`i64` | Signed integers                      |
| `u8`–`u64` | Unsigned integers                    |
| `f32`      | 32-bit float                         |
| `f64`      | 64-bit float                         |
| `bool`     | Boolean                              |
| `cstring`  | Null-terminated C string             |
| `ptr`      | Pointer (fits in 52-bit JS number)   |
| `buffer`   | TypedArray backed by native memory   |
| `function` | Function pointer / callback          |

Use `u64_fast` and `i64_fast` for 64-bit values that fit in a JS number without BigInt overhead.

## Pointers and buffers

Convert between TypedArrays and raw pointers:

```ts
import { ptr, toArrayBuffer, read } from "bun:ffi";

const buf = new Uint8Array(32);
const addr = ptr(buf);

read.u8(addr, 0);
read.i32(addr, 4);

const copy = new DataView(toArrayBuffer(addr, 0, 32));
```

`read.*` provides fast typed reads at byte offsets without creating intermediate objects.

## Callbacks

Pass JavaScript functions to native code with `JSCallback`:

```ts
import { dlopen, JSCallback, CString } from "bun:ffi";

const onMatch = new JSCallback(
  (ptr, len) => /hello/.test(new CString(ptr, len)),
  {
    returns: "bool",
    args: ["ptr", "usize"],
    threadsafe: true,
  },
);

// Pass onMatch.pointer to a C function expecting a callback
onMatch.close(); // free when done
```

Set `threadsafe: true` when the callback may be invoked from non-JS threads.

## Platform notes

The `suffix` helper returns the correct extension per platform: `.dylib` (macOS), `.so` (Linux), `.dll` (Windows). Use it to keep code portable:

```ts
import { suffix } from "bun:ffi";

const lib = dlopen(`libmylib.${suffix}`, { /* ... */ });
```
