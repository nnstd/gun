---
title: Event Loop
lead: Gun includes an event loop runtime that mirrors async semantics from Node.js and Bun.
sections:
  - Runtime entrypoint
---

## Runtime entrypoint

```go
import eventloop "github.com/nnstd/gun/runtime/eventloop"

func main() {
    defer error.RecoverMain()
    // ... transpiled code ...
    eventloop.Default.Run()
}
```
