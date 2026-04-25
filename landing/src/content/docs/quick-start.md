---
title: Quick Start
lead: Transpile your first file in under a minute.
sections:
  - Write some JavaScript
  - Transpile
  - Run
  - Build a binary
---

## Write some JavaScript

```js
import { createServer } from 'http'

const port = 8080

createServer((req, res) => {
  res.writeHead(200)
  res.end('Hello from Go!\n')
}).listen(port)

console.log(`Listening on :${port}`)
```

## Transpile

```bash
gun transpile server.js -o server.go
```

## Run

```bash
go run server.go
# -> Listening on :8080
```

> Use `gun watch` during development to auto-transpile on file changes.

## Build a binary

```bash
go build -o server ./...
./server
```
