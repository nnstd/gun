---
title: Debugging
lead: Debug Gun-built apps by starting with source-level tests, enabling source maps, and narrowing issues to compatibility, configuration, or runtime data.
sections:
  - Start with checks
  - Reproduce locally
  - Use source maps
  - Common causes
---

## Start with checks

Run compatibility checks before debugging generated output:

```bash
gun check src/index.ts
```

Fix check errors first. They often explain why a dependency, import pattern, or API call cannot be built as-is.

## Reproduce locally

Use a clean build:

```bash
rm -rf build/gun
gun transpile src/index.ts -o build/gun --source-maps
go run ./build/gun
```

Keep the same environment variables you use in production or staging.

## Use source maps

Enable source maps when tracking stack traces back to source files:

```bash
gun transpile src/index.ts -o build/gun --source-maps
```

Pair source maps with structured logs that include request IDs, user IDs, or job IDs.

## Common causes

- A dependency uses a native addon.
- A module path is generated dynamically.
- Environment variables differ between local and production.
- A test only covered the Node.js path, not the Gun-built artifact.
- A package relies on undocumented engine behavior.

When in doubt, isolate the failing package or feature behind a small adapter and test that adapter directly.
