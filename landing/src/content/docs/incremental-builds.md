---
title: Incremental Builds
lead: Gun watch mode reuses Tree-sitter incremental parsing so only the modified subtree is reanalyzed and re-emitted.
sections:
  - Performance profile
---

## Performance profile

Cold builds of a 50k-line codebase typically finish in under four seconds. Warm rebuilds after a single-file edit usually land under 20ms.
