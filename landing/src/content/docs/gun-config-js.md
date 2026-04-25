---
title: gun.config.js
lead: Place a gun.config.js at your project root to control entrypoints, output, source maps, and dependency aliases.
sections:
  - Example config
  - Options
---

## Example config

```js
export default {
  entry: 'src/index.js',
  out: 'go/',
  sourceMaps: true,
  module: 'github.com/my/app',
  aliases: {
    lodash: 'github.com/my/lodash-go',
  },
}
```

## Options

- `entry`: Entry point file.
- `out`: Output directory.
- `sourceMaps`: Emit source maps.
- `module`: Go module path.
- `aliases`: npm package to Go module overrides.
