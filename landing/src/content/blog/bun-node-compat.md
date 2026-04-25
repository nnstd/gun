---
title: Node.js and Bun API compatibility in Gun
slug: bun-node-compat
tag: Compat
date: Apr 17, 2026
excerpt: Which built-ins are supported today, what is on the roadmap, and how the compatibility layer stays testable across two ecosystems.
readTime: 5 min
color: '#4dd0e8'
author: Yuki T.
authorRole: Compat
---

Gun ships with a compatibility layer that implements the most commonly used Node.js and Bun APIs. The practical question is coverage, not ideology.

## Supported today

- `http / https`: `createServer`, `request`, `fetch`
- `fs`: `readFile`, `writeFile`, `stat`, `watch`
- `path`: `join`, `resolve`, `dirname`, `basename`
- `console`: `log`, `error`, `warn`, `table`
- `process`: `env`, `argv`, `exit`, `cwd`
- `timers`: `setTimeout`, `setInterval`, `clearTimeout`
- `events`: `EventEmitter`
- `stream`: `Readable`, `Writable`, `Transform`
