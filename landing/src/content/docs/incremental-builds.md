---
title: Incremental Builds
lead: Use watch mode for local rebuilds and clean builds for CI.
sections:
  - Watch mode
  - Rebuild triggers
  - Local workflow
  - CI workflow
---

## Watch mode

```bash
gun watch src/index.ts -o build/gun
```

Watch mode is for development. Keep it running next to your app process or test command.

## Rebuild triggers

Gun watches files reached from the entrypoint. Changes to imported source files should trigger a rebuild. Changes outside that graph may require restarting watch mode or importing the file from the entrypoint path.

## Local workflow

A typical local loop:

```bash
gun watch src/index.ts -o build/gun
go run ./build/gun
npm test
```

If behavior looks stale, stop watch mode and run a clean build:

```bash
rm -rf build/gun
gun transpile src/index.ts -o build/gun
```

## CI workflow

Do not rely on incremental state in CI:

```bash
gun check src/index.ts
gun transpile src/index.ts -o build/gun
go test ./build/gun/...
```

CI should start from a clean checkout and rebuild everything from source.
