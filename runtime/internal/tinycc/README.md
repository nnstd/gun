# Internal TinyCC Wrapper

This package is adapted from `github.com/DiMalovanyy/go-tinycc`, which is
licensed under LGPL-2.1. The upstream wrapper covers basic context creation and
compilation, but leaves relocation and symbol lookup incomplete. Gun keeps this
small internal wrapper so `bun:ffi.cc()` can compile C source through embedded
TinyCC and expose compiled symbols through the existing FFI call path.

TinyCC source and the built `libtcc.a`/runtime support objects are vendored in
`lib/tinycc` so `cc()` does not depend on a system `libtcc` dynamic library.
