import { serve } from "bun";
import { join } from "node:path";
import { existsSync } from "node:fs";

// @ts-ignore — generated at build time
import handler from "./dist/server/server.js";

const clientDir = join(import.meta.dir, "dist/client");
const port = Number(process.env.PORT ?? 3000);

serve({
  port,
  async fetch(req) {
    const url = new URL(req.url);
    const filePath = join(clientDir, url.pathname);

    // Serve static assets from dist/client
    if (existsSync(filePath) && !filePath.endsWith("/")) {
      return new Response(Bun.file(filePath));
    }

    // Fall through to SSR handler (includes /api/*)
    return handler.fetch(req);
  },
});

console.log(`Server running on http://0.0.0.0:${port}`);
