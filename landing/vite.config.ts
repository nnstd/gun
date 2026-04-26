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
        crawlLinks: true,
        routes: ["/", "/docs", "/blog"],
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
