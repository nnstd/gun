import dgram from "node:dgram";

const sender = dgram.createSocket("udp6");
const receiver = dgram.createSocket("udp6");

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
    sender.send("ipv6", addr.port, "::1");
  });
});

receiver.bind(0);
