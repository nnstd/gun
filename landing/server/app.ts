import { Hono } from "hono";

export const app = new Hono().basePath("/api");

const STARS_TTL_MS = 60 * 60 * 1000;
let starsCache: { stars: number; expiresAt: number } | null = null;
let starsPromise: Promise<number> | null = null;

async function fetchStars(): Promise<number> {
  const response = await fetch("https://api.github.com/repos/nnstd/gun", {
    headers: {
      Accept: "application/vnd.github+json",
      "User-Agent": "gun-landing",
    },
  });

  if (!response.ok) {
    throw new Error(`GitHub API returned ${response.status}`);
  }

  const data = (await response.json()) as { stargazers_count?: number };
  return data.stargazers_count ?? 0;
}

app.get("/github/stars", async (c) => {
  const now = Date.now();

  if (starsCache && starsCache.expiresAt > now) {
    return c.json({ stars: starsCache.stars });
  }

  starsPromise ??= fetchStars()
    .then((stars) => {
      starsCache = { stars, expiresAt: Date.now() + STARS_TTL_MS };
      return stars;
    })
    .finally(() => {
      starsPromise = null;
    });

  try {
    const stars = await starsPromise;
    return c.json({ stars });
  } catch {
    if (starsCache) {
      return c.json({ stars: starsCache.stars });
    }
    return c.json({ error: "Failed to fetch GitHub repository stats" }, 502);
  }
});

export type AppType = typeof app;
