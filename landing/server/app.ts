import { Hono } from "hono";

export const app = new Hono().basePath("/api");

app.get("/github/stars", async (c) => {
  const response = await fetch("https://api.github.com/repos/nnstd/gun", {
    headers: {
      Accept: "application/vnd.github+json",
      "User-Agent": "gun-landing",
    },
  });

  if (!response.ok) {
    return c.json({ error: "Failed to fetch GitHub repository stats" }, 502);
  }

  const data = (await response.json()) as { stargazers_count?: number };

  return c.json({ stars: data.stargazers_count ?? 0 });
});

export type AppType = typeof app;
