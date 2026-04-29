---
title: Async / Await
lead: Write asynchronous code with async functions and await, then test the built artifact against real I/O paths.
sections:
  - Basic usage
  - Error handling
  - Parallel work
  - Watchouts
---

## Basic usage

```ts
async function getProfile(id: string) {
  const user = await loadUser(id)
  const settings = await loadSettings(id)
  return { user, settings }
}
```

Gun supports ordinary promise-returning functions and `await` expressions.

## Error handling

Add context at boundaries:

```ts
try {
  await sendEmail(message)
} catch (error) {
  console.error('failed to send email', error)
  throw error
}
```

This makes production logs more useful regardless of where the error originated.

## Parallel work

Use `Promise.all` when operations are independent:

```ts
const [user, teams] = await Promise.all([
  loadUser(id),
  loadTeams(id),
])
```

Avoid accidental parallelism for operations that mutate the same resource.

## Watchouts

- Do not leave rejected promises unhandled.
- Avoid dynamic imports inside hot request paths.
- Prefer explicit startup checks for required services.
- Keep long-running background work observable with logs or metrics.
