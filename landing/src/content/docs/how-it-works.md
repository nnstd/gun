---
title: How It Works
lead: Gun starts from an entrypoint, checks the project graph, and writes an output directory for the final application.
sections:
  - Commands
  - Project graph
  - Dependencies
  - Output
---

## Commands

Most workflows use three commands:

```bash
gun check src/index.ts
gun transpile src/index.ts -o build/gun
gun watch src/index.ts -o build/gun
```

Use `check` before the first build and in pull requests. Use `transpile` for clean builds. Use `watch` while editing locally.

## Project graph

Gun follows imports from the entrypoint:

```ts
import { createServer } from 'http'
import { loadConfig } from './config'
```

Static imports are easiest to validate. Runtime-created module names are harder to check before deployment.

## Dependencies

Ordinary npm packages work best when they use JavaScript, TypeScript, and standard server APIs. Packages with native addons or engine-specific behavior may need an adapter.

Run this after changing dependencies:

```bash
gun check src/index.ts
```

## Output

Treat the output directory as a build artifact:

```bash
rm -rf build/gun
gun transpile src/index.ts -o build/gun
```

Do not edit output files by hand. Commit source files, tests, package files, and scripts instead.
