import http from "node:http";

const srv = http.createServer((req: any, res: any) => {
  res.setHeader("X-Probe", "1");
  res.end("hello");
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  const url = "http://127.0.0.1:" + a.port + "/";
  const req = http.request(url, (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("status=" + resp.statusCode);
      console.log("xprobe=" + resp.headers["x-probe"]);
      console.log("body=" + body);
      srv.close();
    });
  });
  req.end();
});
