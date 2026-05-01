import { defineConfig } from "vite";
import { devtools } from "@tanstack/devtools-vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import contentCollections from "@content-collections/vite";

export default defineConfig({
  resolve: { tsconfigPaths: true },
  plugins: [
    devtools(),
    contentCollections({
      isEnabled: () => process.env.VITEST !== "true",
    }),
    tailwindcss(),
    tanstackStart({
      prerender: {
        enabled: true,
        crawlLinks: false,
        routes: [
          "/", "/blog",
          "/docs/introduction", "/docs/installation", "/docs/quick-start",
          "/docs/how-it-works", "/docs/runtime-semantics", "/docs/event-loop", "/docs/debugging",
          "/docs/variables", "/docs/functions", "/docs/classes", "/docs/async-await", "/docs/modules",
          "/docs/http-server", "/docs/fetch", "/docs/open-telemetry",
          "/docs/npm-dependencies", "/docs/source-maps",
          "/docs/nodejs-compat", "/docs/bun-compat", "/docs/ffi", "/docs/c-compiler",
          "/docs/project-scripts", "/docs/cli-reference",
          "/docs/incremental-builds", "/docs/ci-integration",
        ],
      },
    }),
    viteReact(),
  ],
  server: {
    allowedHosts: true,
  },
  preview: {
    host: "127.0.0.1",
  },
});
