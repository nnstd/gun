---
title: 'Transpiling npm dependencies: the full story'
slug: npm-deps
tag: Feature
date: Apr 14, 2026
excerpt: Gun does not stop at your app code. It walks node_modules and transpiles every package along the way, including the ugly edge cases.
readTime: 7 min
color: '#f5c542'
author: Devin H.
authorRole: Compiler
---

One of Gun's most ambitious features is full npm dependency transpilation. When you run `gun transpile`, it does not stop at your source tree.

## Why this matters

A typical Node.js app has hundreds of dependencies. If only your own files were transpiled, the output would still need a JavaScript runtime to execute the rest of the graph.

```bash
gun transpile src/index.js -o go/
```

## Handling edge cases

Some packages ship native addons. Gun handles that through aliases so those packages can be redirected to hand-written Go equivalents when needed.

```js
export default {
  aliases: {
    bcrypt: 'github.com/my/bcrypt-go',
  },
}
```
