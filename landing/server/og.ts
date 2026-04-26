import { Hono } from "hono";
import satori from "satori";
import { initWasm, Resvg } from "@resvg/resvg-wasm";
import { readFileSync } from "node:fs";
import { join } from "node:path";

export const ogApp = new Hono().basePath("/og");

const geistRegular = readFileSync(
  join(process.cwd(), "src/assets/Geist-Regular.ttf"),
);
const geistBold = readFileSync(
  join(process.cwd(), "src/assets/Geist-Bold.ttf"),
);

let wasmReady: Promise<void>;
if (import.meta.hot) {
  if (!import.meta.hot.data.__resvgInit) {
    import.meta.hot.data.__resvgInit = initWasm(
      readFileSync(
        join(process.cwd(), "node_modules/@resvg/resvg-wasm/index_bg.wasm"),
      ),
    );
  }
  wasmReady = import.meta.hot.data.__resvgInit;
} else {
  wasmReady = initWasm(
    readFileSync(
      join(process.cwd(), "node_modules/@resvg/resvg-wasm/index_bg.wasm"),
    ),
  );
}

ogApp.get("/image.png", async (c) => {
  await wasmReady;

  const svg = await satori(
    {
      type: "div",
      props: {
        style: {
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          background:
            "radial-gradient(circle at 30% 25%, #2a2a72 0%, #0f0f2e 60%, #050518 100%)",
          fontFamily: "Geist",
          gap: "28px",
          padding: "80px",
        },
        children: [
          {
            type: "svg",
            props: {
              width: "120",
              height: "120",
              viewBox: "0 0 200 200",
              fill: "none",
              children: [
                {
                  type: "defs",
                  props: {
                    children: [
                      {
                        type: "radialGradient",
                        props: {
                          id: "og-mark",
                          cx: "40%",
                          cy: "35%",
                          r: "65%",
                          children: [
                            {
                              type: "stop",
                              props: { offset: "0%", "stop-color": "#6666d4" },
                            },
                            {
                              type: "stop",
                              props: { offset: "100%", "stop-color": "#2a2a72" },
                            },
                          ],
                        },
                      },
                    ],
                  },
                },
                {
                  type: "ellipse",
                  props: { cx: "82", cy: "54", rx: "18", ry: "22", fill: "#3535a0" },
                },
                {
                  type: "ellipse",
                  props: { cx: "118", cy: "54", rx: "18", ry: "22", fill: "#3535a0" },
                },
                {
                  type: "circle",
                  props: { cx: "100", cy: "96", r: "80", fill: "url(#og-mark)" },
                },
                {
                  type: "circle",
                  props: { cx: "78", cy: "92", r: "18", fill: "white" },
                },
                {
                  type: "circle",
                  props: { cx: "122", cy: "92", r: "18", fill: "white" },
                },
                {
                  type: "circle",
                  props: { cx: "80", cy: "94", r: "11", fill: "#0f0f2e" },
                },
                {
                  type: "circle",
                  props: { cx: "124", cy: "94", r: "11", fill: "#0f0f2e" },
                },
                {
                  type: "circle",
                  props: { cx: "85", cy: "88", r: "4.5", fill: "white" },
                },
                {
                  type: "circle",
                  props: { cx: "129", cy: "88", r: "4.5", fill: "white" },
                },
                {
                  type: "path",
                  props: {
                    d: "M82 120 Q100 136 118 120",
                    stroke: "#0f0f2e",
                    "stroke-width": "6",
                    fill: "none",
                    "stroke-linecap": "round",
                  },
                },
              ],
            },
          },
          {
            type: "div",
            props: {
              style: {
                fontSize: "120px",
                fontWeight: 700,
                color: "#ffffff",
                letterSpacing: "-4px",
                lineHeight: 1,
              },
              children: "gun",
            },
          },
          {
            type: "div",
            props: {
              style: {
                fontSize: "32px",
                color: "#a0a0ff",
                fontWeight: 400,
                textAlign: "center",
                maxWidth: "900px",
                lineHeight: 1.2,
              },
              children: "Compile JavaScript to Go",
            },
          },
          {
            type: "div",
            props: {
              style: {
                display: "flex",
                gap: "14px",
                marginTop: "12px",
              },
              children: ["TypeScript", "npm", "Bun & Node"].map((label) => ({
                type: "div",
                props: {
                  style: {
                    backgroundColor: "rgba(102, 102, 212, 0.18)",
                    color: "#c8c8ff",
                    fontSize: "20px",
                    fontWeight: 600,
                    padding: "10px 24px",
                    borderRadius: "999px",
                    border: "1px solid rgba(160, 160, 255, 0.35)",
                  },
                  children: label,
                },
              })),
            },
          },
          {
            type: "div",
            props: {
              style: {
                fontSize: "22px",
                color: "#7c7ee8",
                marginTop: "20px",
                fontWeight: 500,
              },
              children: "gun.nnstd.dev",
            },
          },
        ],
      },
    },
    {
      width: 1200,
      height: 630,
      fonts: [
        { name: "Geist", data: geistRegular, weight: 400, style: "normal" as const },
        { name: "Geist", data: geistBold, weight: 700, style: "normal" as const },
      ],
    },
  );

  const resvg = new Resvg(svg, {
    fitTo: { mode: "width", value: 1200 },
  });
  const png = resvg.render().asPng();

  return c.body(png, 200, {
    "Content-Type": "image/png",
    "Cache-Control": "public, max-age=86400, s-maxage=604800",
  });
});
