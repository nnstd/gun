---
title: npm Dependencies
lead: Gun follows your dependency graph and includes packages that can be checked and built ahead of time.
sections:
  - What works best
  - Native packages
  - Adapters
  - Dependency hygiene
---

## What works best

Packages work best when they use:

- Static imports.
- Plain JavaScript or TypeScript.
- Standard Node.js, Bun, or Web APIs.
- Runtime configuration through environment variables or explicit options.

Run this before adopting a package in a Gun-built service:

```bash
gun check src/index.ts
```

## Native packages

Packages with native addons need special handling. Examples include some database drivers, password hashing packages, image libraries, and compression tools.

Use a pure JavaScript package when it is acceptable, or put the package behind an adapter so it can be replaced with a supported implementation.

## Adapters

Wrap compatibility-sensitive packages in a local module:

```ts
// src/adapters/passwords.ts
import bcrypt from 'bcryptjs'

export async function hashPassword(value: string) {
  return bcrypt.hash(value, 12)
}
```

Import the adapter from application code instead of importing the package everywhere. If you need to change the implementation later, the change stays local.

## Dependency hygiene

- Prefer direct dependencies over transitive reach-through imports.
- Keep package versions pinned in lockfiles.
- Run compatibility checks on dependency updates.
- Add smoke tests for code paths that cross package boundaries.
