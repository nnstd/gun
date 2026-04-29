---
title: Source Maps
lead: Enable source maps when stack traces need to point back to the original JavaScript or TypeScript files.
sections:
  - Enable
  - Development
  - Production
  - Tips
---

## Enable

Pass `--source-maps` during build:

```bash
gun transpile src/index.ts -o build/gun --source-maps
```

Add the flag to a package script when every build should include maps:

```json
{
  "scripts": {
    "gun:build": "gun transpile src/index.ts -o build/gun --source-maps"
  }
}
```

## Development

Use maps while testing the built app locally:

```bash
gun transpile src/index.ts -o build/gun --source-maps
go run ./build/gun
```

Keep logs structured so stack traces can be matched to request IDs, job IDs, or user IDs.

## Production

Store source maps with the release that produced them. If source code is private, upload maps to your private error tracker instead of publishing them with public assets.

## Tips

- Enable source maps in preview environments.
- Attach the git revision to logs.
- Keep the binary, maps, and build logs from the same CI run.
- Reproduce production issues from the same commit and package lockfile.
