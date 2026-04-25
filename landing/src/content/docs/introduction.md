---
title: Introduction
lead: Gun is a JavaScript-to-Go transpiler. It converts your JS or TS source and its npm dependencies into valid Go code that runs natively without Node.js in production.
sections:
  - Why Gun?
  - When not to use Gun
---

Unlike source-to-source translators that try to produce idiomatic Go, Gun preserves JavaScript semantics through a lightweight runtime called `jsvalue`. Dynamic typing, prototype chains, and the event loop stay intact.

> Gun is not trying to turn your JS into hand-written Go. It is trying to make your JS run as Go, compiled and deployment-friendly.

## Why Gun?

JavaScript is fast to write. Go is fast to run. Gun gives you both without the rewrite tax of porting a production codebase by hand.

- **10x faster runtime**: Go compiled runtime versus Node.js V8.
- **Zero runtime deps**: No Node.js or Bun required on servers.
- **Node and Bun APIs**: Common built-ins stay available.
- **npm deps included**: Dependencies are transpiled with your app.

## When not to use Gun

Gun is best for server-side JS that wants to ship as a static Go binary. It is not a front-end bundler, and it is not the right fit for runtime `eval` heavy workloads or V8-specific behavior you cannot model statically.
