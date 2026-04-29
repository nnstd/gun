---
title: Node.js Compat
lead: Gun supports the Node.js APIs most backend services reach for first, with compatibility checked during build.
sections:
  - Supported areas
  - HTTP example
  - Filesystem example
  - Compatibility strategy
---

## Supported areas

Gun focuses on practical backend coverage:

- `http` and `https` servers and clients.
- `fs` and `fs/promises` file operations.
- `path`, `url`, `os`, and `process` helpers.
- `events` and stream-style APIs.
- `buffer`, `crypto`, `zlib`, and timers.

Coverage expands based on real packages and applications, not a checklist alone.

## HTTP example

```ts
import { createServer } from 'http'

createServer((req, res) => {
  res.writeHead(200, { 'content-type': 'application/json' })
  res.end(JSON.stringify({ ok: true, path: req.url }))
}).listen(8080)
```

## Filesystem example

```ts
import { readFile } from 'fs/promises'

const config = JSON.parse(await readFile('config.json', 'utf8'))
```

## Compatibility strategy

Run `gun check` in CI. It flags unsupported APIs and dynamic usage before the build reaches production.

If a package uses a native addon or engine-specific behavior, isolate it behind a small adapter module.
