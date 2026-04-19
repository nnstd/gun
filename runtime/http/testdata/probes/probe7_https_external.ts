import https from "node:https";

const req = https.get("https://example.com/", (res: any) => {
  console.log("status=" + res.statusCode);
  console.log("httpVersion=" + res.httpVersion);
  res.on("data", (_c: any) => {});
  res.on("end", () => {});
});
req.on("error", (e: any) => {
  console.log("err=" + e.message);
});
