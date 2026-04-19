import http from "node:http";
import fs from "node:fs";

const sockPath = "/tmp/_gun_probe6.sock";
try { fs.unlinkSync(sockPath); } catch (_e) {}

const srv = http.createServer((req: any, res: any) => {
  res.end("via-unix");
});

srv.listen(sockPath, () => {
  const a: any = srv.address();
  console.log("addr-typeof=" + typeof a);
  console.log("addr=" + a);
  const req = http.request({ socketPath: sockPath, path: "/" }, (resp: any) => {
    let body = "";
    resp.on("data", (c: any) => { body += String(c); });
    resp.on("end", () => {
      console.log("status=" + resp.statusCode);
      console.log("body=" + body);
      srv.close();
    });
  });
  req.end();
});
