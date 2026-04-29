---
title: Event Loop
lead: Gun supports the async patterns backend JavaScript uses most often: promises, async functions, timers, and request callbacks.
sections:
  - Supported patterns
  - Timers
  - Request handlers
  - Practical limits
---

## Supported patterns

Use `async` and `await` for ordinary asynchronous work:

```ts
export async function handler(req) {
  const user = await loadUser(req.url)
  return JSON.stringify(user)
}
```

Promise chains are supported too, but `async` functions usually produce clearer diagnostics.

## Timers

Timer APIs work for common scheduling patterns:

```ts
setTimeout(() => flushMetrics(), 1000)

const interval = setInterval(() => poll(), 5000)
clearInterval(interval)
```

Avoid using timers as a substitute for durable queues. If work must survive process restarts, use a real queue or database-backed job system.

## Request handlers

HTTP handlers can be synchronous or async:

```ts
createServer(async (req, res) => {
  const body = await render(req.url ?? '/')
  res.end(body)
})
```

Keep handler state explicit. Shared mutable globals are harder to reason about after any build step, not just Gun.

## Practical limits

Gun is best with async flows that are visible in the source graph. Runtime-generated functions, ad hoc module loading, and engine-specific scheduling assumptions should be avoided or wrapped behind a small adapter.
