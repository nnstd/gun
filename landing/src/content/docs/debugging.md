---
title: Debugging
lead: Source maps plus go run or delve give you a JS-oriented debugging experience on top of a Go binary.
sections:
  - Step through transpiled code
---

## Step through transpiled code

```bash
dlv debug ./go/server.go

# View the original JS source for any frame
(dlv) source-map
```

> If you hit a runtime error that is hard to trace, run with `GUN_TRACE=1` so each JSValue operation logs its source location.
