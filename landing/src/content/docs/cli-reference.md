---
title: CLI Reference
lead: Use the Gun CLI to check, build, watch, and inspect projects.
sections:
  - gun check
  - gun transpile
  - gun watch
  - gun debug
---

## gun check

Validate an entrypoint:

```bash
gun check src/index.ts
```

Use it in pull requests and before dependency upgrades. It reports unsupported APIs, dynamic module patterns, and dependency issues before a full build.

## gun transpile

Build an entrypoint into an output directory:

```bash
gun transpile src/index.ts -o build/gun
```

Common flags:

```bash
-o, --out <path>       Output directory
--source-maps          Write source maps
```

## gun watch

Rebuild on file changes:

```bash
gun watch src/index.ts -o build/gun
```

Use watch mode locally. Use `gun transpile` from a clean checkout for release builds.

## gun debug

Inspect how Gun sees a file or project:

```bash
gun debug src/index.ts
```

Use debug output when a compatibility error is unclear or a dependency behaves differently than expected.
