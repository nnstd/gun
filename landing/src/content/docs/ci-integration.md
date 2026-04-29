---
title: CI Integration
lead: CI should check compatibility, build from a clean checkout, run tests, and archive the deployable artifact.
sections:
  - Pull requests
  - Release builds
  - GitHub Actions example
  - Caching
---

## Pull requests

Run fast checks on every pull request:

```bash
npm ci
npm test
npx gun check src/index.ts
```

This catches most compatibility issues before merge.

## Release builds

On release branches or deploy previews, run the full build:

```bash
npx gun transpile src/index.ts -o build/gun --source-maps
go test ./build/gun/...
go build -o dist/app ./build/gun
```

Archive the binary, source maps, and build logs together.

## GitHub Actions example

```yaml
name: ci

on: [pull_request, push]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: npm ci
      - run: npm test
      - run: npx gun check src/index.ts
      - run: npx gun transpile src/index.ts -o build/gun --source-maps
      - run: go build -o dist/app ./build/gun
```

## Caching

Cache npm and Go build caches, but do not cache generated output as source of truth. Regenerate it from the current commit.
