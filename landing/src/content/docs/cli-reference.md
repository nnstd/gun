---
title: CLI Reference
lead: Gun ships a small CLI surface focused on transpilation, watch mode, checking, and pipeline debugging.
sections:
  - gun transpile
  - gun watch
  - gun check
  - gun debug
---

## gun transpile

```bash
gun transpile <input> [flags]

Flags:
  -o, --out <path>    Output file or directory
  -w, --watch         Watch for changes
  --source-maps       Emit source maps
  --config <path>     Path to gun.config.js
```

## gun watch

```bash
gun watch src/ -o go/
# -> Watching 142 files
# -> Changed: src/server.js (re-transpiled in 12ms)
```

## gun check

```bash
gun check src/
# -> ✓ 142 files OK
```

## gun debug

```bash
gun debug src/server.js --stage=analyze
```
