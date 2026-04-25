---
title: Installation
lead: Gun ships as an npm package. You need Node.js 18+ or Bun 1.x for the CLI and Go 1.21+ to compile the output.
sections:
  - Prerequisites
  - Install the CLI
  - Verify installation
  - Go runtime module
---

## Prerequisites

```bash
node --version    # -> v20.x or higher
go version        # -> 1.21+
```

## Install the CLI

Pick your package manager. The CLI is pure JavaScript and does not ship as a native binary.

```bash
npm i -g gun-transpiler
# or
bun add -g gun-transpiler
```

## Verify installation

```bash
gun --version
# -> gun v1.0.2
```

> Gun also works as a local dependency. Run it with `npx gun` or through package scripts if you prefer a locked CLI version.

## Go runtime module

The transpiled output imports Gun's Go runtime, so you should add it to your Go module up front.

```bash
go get github.com/nnstd/gun/runtime
```
