---
title: Introduction
lead: Gun builds server-side JavaScript and TypeScript projects into deployable applications.
sections:
  - Install
  - Build
  - Run
  - When to use Gun
---

## Install

Install the CLI globally:

```bash
npm i -g gun-transpiler
```

Or keep it local to a project:

```bash
npm i -D gun-transpiler
```

With Bun:

```bash
bun add -d gun-transpiler
```

## Build

Pass an entrypoint to `gun transpile`:

```bash
gun transpile src/index.ts -o build/gun
```

Run `gun check` first when you are testing a new project or dependency:

```bash
gun check src/index.ts
```

## Run

Run the built app locally:

```bash
go run ./build/gun
```

Build a release binary:

```bash
go build -o dist/app ./build/gun
./dist/app
```

## When to use Gun

Gun is a good fit for:

- HTTP APIs and webhooks.
- Background workers.
- Internal CLIs.
- Services with ordinary npm dependencies.
- Deployments that should not install Node.js or Bun on the target host.

Run compatibility checks before adopting Gun for code that depends on native addons, runtime-created modules, or engine-specific behavior.
