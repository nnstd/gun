---
title: Classes
lead: Classes are supported for ordinary object-oriented code, services, adapters, and small domain models.
sections:
  - Basic classes
  - Fields and methods
  - Inheritance
  - Recommendations
---

## Basic classes

Use classes as you normally would:

```ts
class Counter {
  private value = 0

  next() {
    this.value += 1
    return this.value
  }
}
```

Construct instances with `new` and keep initialization in the constructor or field declarations.

## Fields and methods

Public and private fields are intended for normal application code:

```ts
class TokenStore {
  #tokens = new Map<string, string>()

  set(id: string, token: string) {
    this.#tokens.set(id, token)
  }
}
```

Prefer methods over assigning new function properties to instances at runtime.

## Inheritance

Simple inheritance is supported:

```ts
class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}
```

Keep inheritance shallow. Composition is usually easier to test and easier for compatibility checks.

## Recommendations

- Avoid modifying prototypes at runtime.
- Prefer constructor arguments over global state.
- Keep class fields explicit.
- Test subclasses at the public method level.
