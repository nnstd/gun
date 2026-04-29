---
title: Variables
lead: Use standard JavaScript and TypeScript variable declarations. Gun follows normal scope and closure behavior for backend code.
sections:
  - Declarations
  - Scope
  - Closures
  - Recommendations
---

## Declarations

Prefer `const` by default and `let` when a value changes:

```ts
const port = Number(process.env.PORT ?? 8080)
let activeConnections = 0
```

`var` is supported for compatibility, but new code should avoid it.

## Scope

Block scope works as expected:

```ts
if (enabled) {
  const label = 'enabled'
  console.log(label)
}
```

Avoid relying on subtle hoisting behavior. It makes code harder to check and harder for readers to audit.

## Closures

Closures are supported:

```ts
function counter() {
  let value = 0
  return () => ++value
}
```

Use closures for local state, but prefer explicit objects or classes for state shared across modules.

## Recommendations

- Keep module-level state small.
- Read environment variables at startup.
- Avoid mutating imported objects from many places.
- Prefer plain data over dynamic property mutation when you can.
