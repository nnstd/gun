---
title: HTTP Server
lead: Serve HTTP and HTTPS requests using Bun.serve or Node.js http.createServer.
sections:
  - Bun.serve
  - Node.js http.createServer
  - HTTPS
  - Request handling
  - Streaming
---

## Bun.serve

Use `Bun.serve` for compact HTTP services with a fetch-based handler:

```ts
Bun.serve({
  port: 3000,
  fetch(req) {
    const url = new URL(req.url)
    return new Response(`hello ${url.pathname}`)
  },
})
```

The handler receives a standard `Request` and returns a `Response`. Supports async handlers that return promises.

## Node.js http.createServer

Use the Node.js `http` module for event-based request handling:

```ts
import http from "http"

const server = http.createServer((req, res) => {
  res.writeHead(200, { "Content-Type": "text/plain" })
  res.end("hello world")
})

server.listen(3000)
```

Gun supports the full `http` and `https` API including `ServerResponse`, `IncomingMessage`, event emitters, and `http.request` / `http.get` for outgoing calls.

## HTTPS

Create an HTTPS server with TLS certificates:

```ts
import https from "https"
import fs from "fs"

const server = https.createServer(
  {
    key: fs.readFileSync("key.pem"),
    cert: fs.readFileSync("cert.pem"),
  },
  (req, res) => {
    res.writeHead(200)
    res.end("secure hello")
  },
)

server.listen(443)
```

## Request handling

Read request body, headers, and method:

```ts
Bun.serve({
  fetch(req) {
    const body = await req.json()
    const auth = req.headers.get("authorization")
    return Response.json({ received: body, method: req.method })
  },
})
```

## Streaming

Return streaming responses for large payloads:

```ts
Bun.serve({
  fetch(req) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("chunk 1\n")
        controller.enqueue("chunk 2\n")
        controller.close()
      },
    })
    return new Response(stream, {
      headers: { "Content-Type": "text/plain" },
    })
  },
})
```
