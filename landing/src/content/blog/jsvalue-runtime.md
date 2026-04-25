---
title: 'Inside the JSValue runtime: how Gun preserves JS semantics'
slug: jsvalue-runtime
tag: Deep Dive
date: Apr 8, 2026
excerpt: Go is statically typed. JavaScript is not. Here is how a single tagged-union type bridges the gap without a VM or interpreter.
readTime: 9 min
color: '#4eca8a'
author: Mira P.
authorRole: Runtime
---

Go is statically typed. JavaScript is not. Bridging those worlds without a full interpreter requires a careful design. This is the story of `*jsvalue.JSValue`.

## The core type

Every value in Gun-transpiled code, numbers, strings, arrays, objects, and functions, becomes a tagged union carrying its JavaScript type alongside its Go storage.

```go
var n = jsvalue.NewNumber(float64(42))
var s = jsvalue.NewString("hello")
var b = jsvalue.NewBool(true)
var o = jsvalue.ObjectFrom(map[string]any{
    "name": jsvalue.NewString("gun"),
})
```

## Property access

Dynamic property access is handled via `.Get()` and `.Set()`. Method calls use `.MethodCall()`, which mirrors JavaScript lookup semantics closely enough for real application code.
