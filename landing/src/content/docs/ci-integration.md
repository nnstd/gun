---
title: CI Integration
lead: A typical CI flow runs gun check on pull requests and gun transpile plus go build on release or preview branches.
sections:
  - Example workflow
---

## Example workflow

```bash
# .github/workflows/ci.yml
- run: npm i -g gun-transpiler
- run: gun check src/
- run: gun transpile src/ -o go/
- run: go build -o bin/server ./go/...
```
