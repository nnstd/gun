---
title: Modules
lead: Gun works best with explicit ES module imports and a stable project graph.
sections:
  - Imports
  - Exports
  - Dynamic loading
  - Package boundaries
---

## Imports

Use static imports for local files and packages:

```ts
import { createServer } from 'http'
import { loadConfig } from './config'
```

Static imports let Gun check the whole graph before build time.

## Exports

Named exports are easiest to inspect and test:

```ts
export function normalizePath(value: string) {
  return value.replace(/\\+/g, '/')
}
```

Default exports are supported, but use them intentionally for a module's primary value.

## Dynamic loading

Runtime-generated module names are hard to validate:

```ts
await import(`./plugins/${name}.js`)
```

Prefer an explicit registry:

```ts
const plugins = { csv, json, xml }
const plugin = plugins[name]
```

## Package boundaries

Keep adapters around packages that touch the filesystem, network, or native behavior. A small adapter makes compatibility changes local.
