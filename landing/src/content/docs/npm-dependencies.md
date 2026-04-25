---
title: npm Dependencies
lead: Gun transpiles your dependencies along with your source so the final Go binary stays self-contained.
sections:
  - Why this matters
  - Handling edge cases
---

## Why this matters

If Gun only transpiled your own code, the output would still need a JavaScript runtime for the rest of the graph. Full dependency transpilation is what lets the final binary stand alone.

```bash
gun transpile src/index.js -o go/
```

## Handling edge cases

Packages with native addons can be mapped to hand-written Go equivalents through `gun.config.js` aliases.

```js
export default {
  aliases: {
    bcrypt: 'github.com/my/bcrypt-go',
  },
}
```
