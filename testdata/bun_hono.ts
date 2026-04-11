import { Hono } from "hono";

const app = new Hono();
const port = Number(process.env.PORT || 3011);

app.get("/", (c) => c.text("Hono!"));

Bun.serve({
  port,
  fetch: app.fetch,
});

console.log(`Listening on ${port}`);
