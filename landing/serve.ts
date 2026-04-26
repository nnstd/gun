import { serve } from "bun";
import { join } from "node:path";
import { statSync } from "node:fs";

// @ts-ignore — generated at build time
import handler from "./dist/server/server.js";

const clientDir = join(import.meta.dir, "dist/client");
const port = Number(process.env.PORT ?? 3000);

function resolveStatic(pathname: string): string | null {
  const filePath = join(clientDir, pathname);

  let stat;
  try {
    stat = statSync(filePath);
  } catch {
    return null;
  }

  if (stat.isFile()) return filePath;

  if (stat.isDirectory()) {
    const indexPath = join(filePath, "index.html");
    try {
      if (statSync(indexPath).isFile()) return indexPath;
    } catch {}
  }

  return null;
}

serve({
  port,
  async fetch(req) {
    const url = new URL(req.url);
    const staticPath = resolveStatic(url.pathname);

    if (staticPath) return new Response(Bun.file(staticPath));

    return handler.fetch(req);
  },
});

console.log(`Server running on http://0.0.0.0:${port}`);
