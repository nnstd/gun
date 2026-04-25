---
title: How It Works
lead: Gun runs a compiler pipeline with parse, analysis, emission, and linking phases instead of embedding a JS runtime in production.
sections:
  - Compiler stages
---

## Compiler stages

1. **Parse**: Gun walks your JS or TS source using Tree-sitter and builds a concrete syntax tree for every file, including npm dependencies.
2. **Analyze**: A type-flow pass resolves variable scopes, infers types from usage, and maps JS constructs to Go equivalents.
3. **Emit**: The Go emitter outputs valid Go code while values flow through the JSValue runtime to preserve JavaScript semantics.
4. **Link**: Import paths are rewritten, `go.mod` is updated, and the output is formatted with `gofmt`.
