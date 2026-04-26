import { createFileRoute } from "@tanstack/react-router";
import { ogApp } from "../../../server/og";

const serve = async ({ request }: { request: Request }) => {
  return ogApp.fetch(request);
};

export const Route = createFileRoute("/og/$")({
  server: {
    handlers: {
      GET: serve,
    },
  },
});
