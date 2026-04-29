---
title: Functions
lead: Gun supports ordinary functions, arrow functions, callbacks, closures, and async functions used in backend JavaScript.
sections:
  - Function forms
  - Callbacks
  - Async functions
  - Patterns to avoid
---

## Function forms

Function declarations and arrow functions are supported:

```ts
function formatUser(user) {
  return `${user.id}:${user.name}`
}

const isActive = (user) => user.active === true
```

Use whichever form makes ownership and `this` behavior clearest.

## Callbacks

Callbacks are common in Node-style APIs and array methods:

```ts
items
  .filter((item) => item.enabled)
  .map((item) => item.name)
```

For public module APIs, prefer named functions when the callback is reused or tested independently.

## Async functions

Async functions work for request handlers, setup routines, and dependency calls:

```ts
export async function loadSettings() {
  const raw = await readFile('settings.json', 'utf8')
  return JSON.parse(raw)
}
```

Make failures explicit with `try` / `catch` near the boundary where you can add context.

## Patterns to avoid

Avoid runtime function construction:

```ts
new Function('return process.env.SECRET')
```

Prefer explicit imports and named functions. They are easier for Gun to check and easier for your team to review.
