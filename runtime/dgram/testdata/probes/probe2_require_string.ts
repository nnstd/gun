const dgram = require("dgram");

const sender = dgram.createSocket("udp4");
const receiver = dgram.createSocket("udp4");

receiver.on("message", (msg: any, rinfo: any) => {
  console.log("message=" + msg.toString());
  console.log("family=" + rinfo.family);
  receiver.close(() => {
    console.log("receiver=closed");
  });
  sender.close(() => {
    console.log("sender=closed");
  });
});

receiver.on("listening", () => {
  const addr = receiver.address();
  sender.bind(0, () => {
    sender.send("pong", addr.port, "127.0.0.1");
  });
});

receiver.bind(0);
