---
title: Bun Compat
lead: Gun supports common Bun-style server APIs and keeps Bun-specific usage visible during compatibility checks.
sections:
  - Bun.serve
  - Request and response patterns
  - Environment variables
  - When to add an adapter
---

## Bun.serve

Use `Bun.serve` for compact HTTP services:

```ts
Bun.serve({
  port: 8080,
  fetch(req) {
    return new Response(`hello ${new URL(req.url).pathname}`)
  },
})
```

Gun targets the server-side subset used by application code: request handling, responses, route-like dispatch, and environment configuration.

## Request and response patterns

Prefer standard Web APIs where possible:

```ts
async function fetch(req: Request) {
  const body = await req.json()
  return Response.json({ received: body })
}
```

These patterns are easier to test across Bun, Node-compatible runtimes, and Gun builds.

## Environment variables

Read environment values at startup or request boundaries:

```ts
const token = process.env.API_TOKEN
```

Validate required variables before accepting traffic.

## When to add an adapter

If your code uses Bun-only APIs deeply, add a small adapter module:

```ts
export function serve(options) {
  return Bun.serve(options)
}
```

That keeps compatibility work local if the implementation needs to differ later.
