import http from "node:http";

const srv = http.createServer((req: any, res: any) => {
  res.end("getter");
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  http.get("http://127.0.0.1:" + a.port + "/g", (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("status=" + resp.statusCode);
      console.log("body=" + body);
      srv.close();
    });
  });
});
