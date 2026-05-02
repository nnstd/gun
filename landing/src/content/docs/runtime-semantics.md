---
title: Runtime Semantics
lead: Gun keeps common JavaScript behavior intact for backend code.
sections:
  - Values
  - Objects and arrays
  - Functions
  - Errors
  - What to test
---

## Values

Use normal JavaScript values:

```ts
const id = 42
const name = 'Ada'
const enabled = true
const missing = undefined
```

Nullish checks, booleans, strings, numbers, and object references follow JavaScript semantics.

## Objects and arrays

Objects and arrays work for ordinary data handling:

```ts
const user = { id: 42, name: 'Ada' }
const names = users.map((user) => user.name)
const keys = Object.keys(user)
```

Computed property names are fine when the property names are ordinary runtime values:

```ts
const header = 'content-type'
headers[header] = 'application/json'
```

## Functions

Functions, callbacks, closures, and methods are supported:

```ts
const normalize = (value: string) => value.trim().toLowerCase()
items.map((item) => normalize(item.name))
```

Avoid constructing functions from strings. Prefer explicit imports and named functions.

## Errors

Throw and catch errors normally:

```ts
try {
  await loadConfig()
} catch (error) {
  console.error('failed to load config', error)
  throw error
}
```

Enable source maps when you need production stack traces to point back to source files.

## What to test

Test behavior at the application boundary:

- Request and response handling.
- Error paths.
- Timers and async flows.
- Dependency adapters.
- Environment configuration.
