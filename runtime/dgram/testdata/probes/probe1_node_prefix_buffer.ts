import dgram from "node:dgram";

const { Buffer } = require("buffer");
const sender = dgram.createSocket("udp4");
const receiver = dgram.createSocket("udp4", (msg: any, rinfo: any) => {
  console.log("message=" + msg.toString());
  console.log("family=" + rinfo.family);
  console.log("address=" + rinfo.address);
  console.log("size=" + rinfo.size);
  receiver.close(() => {
    console.log("receiver=closed");
  });
  sender.close(() => {
    console.log("sender=closed");
  });
});

receiver.on("listening", () => {
  const addr = receiver.address();
  console.log("listenFamily=" + addr.family);
  sender.bind(0, () => {
    sender.send(Buffer.from("ping"), addr.port, "127.0.0.1");
  });
});

receiver.bind(0);
