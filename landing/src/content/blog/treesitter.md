---
title: Why we use Tree-sitter for parsing JavaScript
slug: treesitter
tag: Internals
date: Apr 19, 2026
excerpt: Tree-sitter gives Gun incremental parsing, error recovery, and embedded-language support, all of which matter when watch mode is part of the product.
readTime: 8 min
color: '#f472b6'
author: Mira P.
authorRole: Runtime
---

Choosing a parser is one of the most consequential decisions in a transpiler's design. We evaluated several options before settling on Tree-sitter.

## Why Tree-sitter

Incremental parsing, error recovery, and embeddable grammars matter a lot when the same engine powers both cold builds and watch-mode rebuilds. Tree-sitter handles that without dragging a language server into the compiler core.
