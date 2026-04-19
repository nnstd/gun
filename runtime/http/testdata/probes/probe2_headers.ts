import http from "node:http";

const srv = http.createServer((req: any, res: any) => {
  const u = req.url;
  const ua = req.headers["x-test-ua"];
  res.setHeader("Content-Type", "text/plain");
  res.end("url=" + u + " ua=" + ua);
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  const req = http.request({
    hostname: "127.0.0.1",
    port: a.port,
    path: "/probe?x=1",
    headers: { "X-Test-UA": "gun-probe" },
  }, (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("ct=" + resp.headers["content-type"]);
      console.log(body);
      srv.close();
    });
  });
  req.end();
});
