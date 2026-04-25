---
title: 'Introducing Gun: JavaScript to Go transpiler'
slug: introducing-gun
tag: Announcement
date: Apr 2, 2026
excerpt: We are open-sourcing Gun, a tool that compiles JavaScript and TypeScript plus their npm dependencies into valid Go code that ships as a single static binary.
readTime: 6 min
color: '#a0a0ff'
author: Sasha K.
authorRole: Founder
featured: true
---

Today we are open-sourcing **Gun**, a JavaScript-to-Go transpiler that converts your entire JS or TS codebase, including npm dependencies, into valid compilable Go.

## The problem

JavaScript is fast to write, but Node.js still carries startup, runtime, and memory overhead. Rewriting a service in Go yields huge wins, but it is expensive and usually means abandoning the npm ecosystem that made the original system practical.

Gun's answer is simple: do not rewrite. Transpile.

## How it works

Gun uses Tree-sitter to parse the source, runs a type-flow analysis pass, then emits Go that uses the `jsvalue` runtime to preserve JavaScript semantics.

```bash
npm i -g gun-transpiler
gun transpile server.js -o server.go
go run server.go
```

> Gun transpiles npm dependencies too, not just your own code. If your app uses `express` or `axios`, those move through the pipeline with the rest of the graph.

## What's next

We are working on broader Bun API coverage, faster watch-mode rebuilds, and more source map polish. The current landing rewrite is part of making that surface more honest.
