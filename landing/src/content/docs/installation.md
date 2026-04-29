---
title: Installation
lead: Install the Gun CLI with npm or Bun, then verify it from your terminal.
sections:
  - Requirements
  - npm
  - Bun
  - Verify
  - Add scripts
---

## Requirements

Check the local tools:

```bash
node --version
npm --version
go version
```

Recommended versions:

- Node.js 18 or newer, or Bun 1.x.
- Go 1.21 or newer.
- A project entrypoint such as `src/index.ts`, `src/server.ts`, or `server.js`.

## npm

Install globally:

```bash
npm i -g gun-transpiler
```

Install per project:

```bash
npm i -D gun-transpiler
```

Use `npx` when the CLI is local:

```bash
npx gun check src/index.ts
```

## Bun

Install per project:

```bash
bun add -d gun-transpiler
```

Use `bunx` when needed:

```bash
bunx gun check src/index.ts
```

## Verify

```bash
gun --version
gun help
```

If `gun` is not found, use the package runner for your install style:

```bash
npx gun --version
bunx gun --version
```

## Add scripts

Add repeatable commands to `package.json`:

```json
{
  "scripts": {
    "gun:check": "gun check src/index.ts",
    "gun:build": "gun transpile src/index.ts -o build/gun",
    "gun:watch": "gun watch src/index.ts -o build/gun"
  }
}
```

Run the scripts in CI and during local development so every environment uses the same entrypoint and output path.
