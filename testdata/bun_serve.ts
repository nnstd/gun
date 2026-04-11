const port = Number(process.env.PORT || 3010);

Bun.serve({
  port,
  fetch() {
    return new Response("bun-ok");
  },
});

console.log(`Listening on ${port}`);
