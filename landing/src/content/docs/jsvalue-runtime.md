---
title: JSValue Runtime
lead: Every value in transpiled code becomes a JSValue so the generated Go can preserve JavaScript behavior precisely.
sections:
  - Constructors
  - Property access
---

## Constructors

```go
jsvalue.NewNumber(float64(42))
jsvalue.NewString("hello")
jsvalue.NewBool(true)
jsvalue.NewFunction(myFunc)
jsvalue.ObjectFrom(map[string]any{"key": val})
```

## Property access

```go
obj.Get("key")             // obj.key
obj.Set("key", val)        // obj.key = val
obj.MethodCall("fn", arg)  // obj.fn(arg)
fn.Call(arg1, arg2)         // fn(arg1, arg2)
```

> You normally do not call the runtime directly. Gun emits these helpers for you when it lowers JavaScript operations into Go.
