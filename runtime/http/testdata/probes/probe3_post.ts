import http from "node:http";

const srv = http.createServer((req: any, res: any) => {
  let buf = "";
  req.on("data", (c: any) => { buf += String(c); });
  req.on("end", () => {
    res.setHeader("X-Saw", String(buf.length));
    res.end("got:" + buf);
  });
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  const req = http.request({
    method: "POST",
    hostname: "127.0.0.1",
    port: a.port,
    path: "/post",
    headers: { "Content-Type": "text/plain" },
  }, (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("saw=" + resp.headers["x-saw"]);
      console.log(body);
      srv.close();
    });
  });
  req.write("alpha-");
  req.end("omega");
});
