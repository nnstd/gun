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
      pages: [
        { path: "/" },
        { path: "/blog" },
        { path: "/docs/introduction" }, { path: "/docs/installation" }, { path: "/docs/quick-start" },
        { path: "/docs/how-it-works" }, { path: "/docs/runtime-semantics" }, { path: "/docs/event-loop" }, { path: "/docs/debugging" },
        { path: "/docs/variables" }, { path: "/docs/functions" }, { path: "/docs/classes" }, { path: "/docs/async-await" }, { path: "/docs/modules" },
        { path: "/docs/http-server" }, { path: "/docs/fetch" }, { path: "/docs/open-telemetry" },
        { path: "/docs/npm-dependencies" }, { path: "/docs/source-maps" },
        { path: "/docs/nodejs-compat" }, { path: "/docs/bun-compat" }, { path: "/docs/ffi" }, { path: "/docs/c-compiler" },
        { path: "/docs/project-scripts" }, { path: "/docs/cli-reference" },
        { path: "/docs/incremental-builds" }, { path: "/docs/ci-integration" },
      ],
      prerender: {
        enabled: true,
        crawlLinks: false,
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
