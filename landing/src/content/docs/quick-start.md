---
title: Quick Start
lead: Build a small HTTP server with Gun.
sections:
  - Create a server
  - Check compatibility
  - Build with Gun
  - Run locally
  - Build a binary
---

## Create a server

Create `server.ts`:

```ts
import { createServer } from 'http'

const port = Number(process.env.PORT ?? 8080)

createServer((req, res) => {
  res.writeHead(200, { 'content-type': 'text/plain' })
  res.end(`hello from ${req.url}\n`)
}).listen(port)

console.log(`listening on :${port}`)
```

## Check compatibility

```bash
gun check server.ts
```

Fix check errors before building. They usually point to unsupported APIs, dynamic imports, or dependency behavior that needs an adapter.

## Build with Gun

```bash
gun transpile server.ts -o build/gun
```

Use the same pattern for a project entrypoint:

```bash
gun transpile src/index.ts -o build/gun
```

## Run locally

```bash
go run ./build/gun
```

For a faster edit loop, keep watch mode running:

```bash
gun watch server.ts -o build/gun
```

## Build a binary

```bash
go build -o app ./build/gun
./app
```

The binary is the deployable artifact. Build it from a clean checkout in CI for release.
