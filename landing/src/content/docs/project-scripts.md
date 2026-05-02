---
title: Project Scripts
lead: Configure Gun with package scripts and CLI flags.
sections:
  - Package scripts
  - Entrypoints
  - Output paths
  - Environment
---

## Package scripts

Put the commands your team runs in `package.json`:

```json
{
  "scripts": {
    "gun:check": "gun check src/index.ts",
    "gun:build": "gun transpile src/index.ts -o build/gun --source-maps",
    "gun:watch": "gun watch src/index.ts -o build/gun"
  }
}
```

Scripts make the build contract visible in code review.

## Entrypoints

Pass the entrypoint directly:

```bash
gun check src/server.ts
gun transpile src/server.ts -o build/gun
```

For multiple deployables, create one script per entrypoint:

```json
{
  "scripts": {
    "gun:build:api": "gun transpile src/api.ts -o build/api",
    "gun:build:worker": "gun transpile src/worker.ts -o build/worker"
  }
}
```

## Output paths

Use a directory that can be deleted safely:

```bash
rm -rf build/gun
gun transpile src/index.ts -o build/gun
```

Keep source files and output files separate.

## Environment

Read runtime settings from environment variables:

```ts
const port = Number(process.env.PORT ?? 8080)
const databaseUrl = process.env.DATABASE_URL
```

Validate required values during startup so broken deploys fail before accepting traffic.
