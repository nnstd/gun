import http from "node:http";

const srv = http.createServer((req: any, res: any) => {
  res.statusCode = 201;
  res.setHeader("X-Method", req.method);
  res.end("done");
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  const req = http.request({
    method: "PUT",
    hostname: "127.0.0.1",
    port: a.port,
    path: "/r",
  }, (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("status=" + resp.statusCode);
      console.log("method=" + resp.headers["x-method"]);
      console.log("body=" + body);
      srv.close();
    });
  });
  req.end();
});
