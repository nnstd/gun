import http from "node:http";

const blocker = http.createServer((_req: any, _res: any) => {});
blocker.listen(0, "127.0.0.1", () => {
  const a: any = blocker.address();
  const port = a.port;
  const srv = http.createServer((_req: any, _res: any) => {});
  srv.on("error", (e: any) => {
    console.log("code=" + e.code);
    console.log("syscall=" + e.syscall);
    blocker.close();
  });
  srv.listen(port, "127.0.0.1");
});
