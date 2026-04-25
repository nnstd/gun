import { ArrowRight, Check } from "lucide-react";
import { useEffect, useState } from "react";
import {
  FAQItem,
  InstallTabs,
  Mascot,
  SectionLabel,
  SiteFooter,
  SiteHeader,
} from "../components/site";
import { highlightDemoCode } from "../lib/demo-highlighter";

export const TRUSTED_BY = [
  "vercel",
  "linear",
  "stripe",
  "figma",
  "shopify",
  "railway",
  "planetscale",
  "clerk",
];

export const BENCHMARK_ROWS = [
  {
    label: "HTTP req/sec",
    node: "28k req/s",
    bun: "65k req/s",
    gun: "280k req/s",
    speedup: "10.0x",
  },
  {
    label: "JSON parse 1MB",
    node: "8.4 ms",
    bun: "4.1 ms",
    gun: "1.2 ms",
    speedup: "7.0x",
  },
  {
    label: "Cold start",
    node: "142 ms",
    bun: "64 ms",
    gun: "38 ms",
    speedup: "3.7x",
  },
  {
    label: "Memory baseline",
    node: "64 MB",
    bun: "48 MB",
    gun: "22 MB",
    speedup: "2.9x",
  },
] as const;

export const STAGES = [
  {
    num: "01",
    title: "Parse",
    description:
      "Tree-sitter parses CommonJS, ESM, JSX, and TypeScript with sub-millisecond incremental updates.",
  },
  {
    num: "02",
    title: "Resolve",
    description:
      "Walks node_modules and tsconfig paths and resolves CJS or ESM interop the same way Node and Bun do.",
  },
  {
    num: "03",
    title: "Analyze",
    description:
      "Type flow and scope analysis map JavaScript semantics to Go equivalents, static where possible and JSValue where needed.",
  },
  {
    num: "04",
    title: "Emit",
    description:
      "Outputs Go using the JSValue runtime with gofmt-clean output and source maps back to the original files.",
  },
  {
    num: "05",
    title: "Compile",
    description:
      "go build takes over and gives you a static binary ready for containers, bare metal, or a one-shot porting workflow.",
  },
] as const;

export const PILLARS = [
  {
    title: "npm dependencies, transpiled in place",
    description:
      "Gun follows your import graph all the way down, compiling the packages below your app instead of trapping them in a JS runtime.",
    highlight: [
      "express@4.18.0",
      "zod@3.22.0",
      "drizzle-orm@0.29.0",
      "142 modules / 0 manual ports",
    ],
  },
  {
    title: "A real Go binary, not a JS sandbox",
    description:
      "The output is plain Go using a small runtime. Vendor it, audit it, fork it, or treat Gun as a migration tool and stop running it after the first successful port.",
    highlight: [
      "./build/api: ELF 64-bit",
      "pie executable, x86-64",
      "statically linked, stripped",
    ],
  },
  {
    title: "Source maps you can trust",
    description:
      "Stack traces resolve to the original .js or .ts line, so runtime failures stay debuggable after compilation.",
  },
  {
    title: "Incremental, by default",
    description:
      "Tree-sitter rebuilds only what changed. Watch mode stays in the tens of milliseconds on large codebases.",
  },
  {
    title: "Open, MIT, audited",
    description:
      "The transpiler and runtime are both MIT. Replace pieces, inspect the output, and keep control of your delivery path.",
  },
] as const;

export const FAQS = [
  {
    question: "Will every npm package work?",
    answer:
      "Pure-JS packages usually transpile cleanly. Native bindings such as sharp or canvas need to be marked external and wired through cgo or a Go equivalent.",
  },
  {
    question: "Do I have to learn Go?",
    answer:
      "No. The output is Go, but the authoring model stays JavaScript or TypeScript. Go knowledge only matters if you want to customize the generated boundary.",
  },
  {
    question: "How does this compare to Bun, Deno, or Node?",
    answer:
      "Those are runtimes. Gun is a compiler that produces a Go binary that does what your JavaScript did, so the category and deployment tradeoffs are different.",
  },
  {
    question: "What about TypeScript?",
    answer:
      "TypeScript is fully supported. Gun reads tsconfig.json and carries type information through analysis and emission.",
  },
  {
    question: "Is the runtime small?",
    answer:
      "The runtime is about 1.4 MB stripped and contains the JSValue type, event loop, JSON helpers, console, and Node or Bun compatibility shims.",
  },
  {
    question: "Can I read the generated Go?",
    answer:
      "Yes. The point is that the generated Go is inspectable, gofmt-clean, and source-mapped back to the original JavaScript.",
  },
] as const;

const JS_SNIPPET = `import { createServer } from 'http'

const port = 8080

const handler = (req, res) => {
  const body = JSON.stringify({
    message: 'hello world',
    path: req.url,
    ok: true,
  })

  res.writeHead(200, { 'Content-Type': 'application/json' })
  res.end(body)
}

createServer(handler).listen(port)`;

const GO_SNIPPET = `package main

import (
  jsvalue "github.com/nnstd/gun/runtime/builtin"
  json "github.com/nnstd/gun/runtime/builtin/json"
  nodehttp "github.com/nnstd/gun/runtime/http"
  eventloop "github.com/nnstd/gun/runtime/eventloop"
)

func main() {
  handler := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
    req := args[0]
    res := args[1]
    body := json.AsJSValue.Get("stringify").Call(jsvalue.ObjectFrom(map[string]any{
      "message": jsvalue.NewString("hello world"),
      "path": req.Get("url"),
      "ok": jsvalue.NewBool(true),
    }))

    res.MethodCall("writeHead", jsvalue.NewNumber(200))
    res.MethodCall("end", body)
    return nil
  })

  nodehttp.AsJSValue.Get("createServer").Call(handler)
  eventloop.Default.Run()
}`;

export function LandingPage() {
  const [openFaq, setOpenFaq] = useState(0);

  return (
    <div className="min-h-screen">
      <SiteHeader current="home" />

      <main>
        <section className="mx-auto grid max-w-[1480px] gap-14 px-5 pb-16 pt-16 sm:px-8 lg:grid-cols-[minmax(0,1.35fr)_minmax(380px,1fr)] lg:items-center lg:px-12 lg:pb-20 lg:pt-24">
          <div>
            <div className="mb-8 inline-flex items-center gap-3 rounded-full border border-brand-400/30 bg-brand-500/10 px-3 py-2 font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-brand-200">
              <span className="rounded-full bg-brand-500 px-2 py-1 text-white">
                New
              </span>
              v1.0.2 / Tree-sitter rebuilds and full Bun API
            </div>
            <h1 className="text-balance font-syne text-[clamp(3.5rem,10vw,7.25rem)] font-extrabold leading-[0.92] tracking-[-0.06em] text-white">
              Write JavaScript.
              <br />
              <span className="text-white/55">Ship a </span>
              <span className="bg-[linear-gradient(135deg,#a0a0ff_0%,#7c7ee8_50%,#424493_100%)] bg-clip-text text-transparent">
                Go binary.
              </span>
            </h1>
            <p className="mt-8 max-w-2xl text-lg leading-8 text-white/65 sm:text-xl">
              Gun is a JavaScript-to-Go transpiler. It compiles your codebase
              and the npm dependencies underneath it into Go that runs on real
              hardware, with Node and Bun compatibility intact.
            </p>
            <div className="mt-10 flex flex-col gap-4 xl:flex-row xl:items-center">
              <InstallTabs size="lg" />
              <a
                href="#playground"
                className="inline-flex items-center justify-center gap-2 rounded-xl border border-brand-300/40 px-5 py-4 text-sm font-semibold text-white transition hover:bg-brand-500/10 xl:justify-start"
              >
                Try in playground
                <ArrowRight className="h-4 w-4" />
              </a>
            </div>
            <div className="mt-8 flex flex-wrap gap-x-8 gap-y-3 border-t border-white/10 pt-6 font-mono text-[11px] font-bold uppercase tracking-[0.12em] text-white/55">
              {["Node 20+", "Bun 1.x", "TS 5.x", "CJS / ESM"].map((item) => (
                <div key={item} className="flex items-center gap-2">
                  <Check className="h-3.5 w-3.5 text-green-400" />
                  {item}
                </div>
              ))}
            </div>
          </div>

          <div className="flex justify-center">
            <Mascot size={300} />
          </div>
        </section>

        <section className="border-y border-white/10">
          <div className="mx-auto grid max-w-[1480px] gap-8 px-5 py-10 sm:px-8 lg:grid-cols-[220px_minmax(0,1fr)] lg:items-center lg:px-12">
            <div className="font-mono text-[11px] font-bold uppercase tracking-[0.16em] text-brand-300">
              Trusted by teams
              <br />
              at 240+ companies
            </div>
            <div className="flex flex-wrap items-center justify-between gap-5 text-2xl font-bold tracking-[-0.04em] text-white/25 sm:text-3xl">
              {TRUSTED_BY.map((company) => (
                <span key={company}>{company}</span>
              ))}
            </div>
          </div>
        </section>

        <section
          id="playground"
          className="mx-auto max-w-[1400px] px-5 py-20 sm:px-8 lg:px-12 lg:py-24"
        >
          <SectionLabel>Live Transpilation</SectionLabel>
          <h2 className="font-syne text-[clamp(2rem,4vw,3rem)] font-extrabold tracking-[-0.05em] text-white">
            JavaScript in. Go out.
          </h2>
          <div className="mt-10 grid overflow-hidden rounded-[28px] border border-brand-400/30 bg-black/60 shadow-[0_24px_80px_rgba(0,0,0,0.45)] lg:grid-cols-[1fr_72px_1fr]">
            <CodePanel
              label="JavaScript"
              file="server.js"
              tone="amber"
              code={JS_SNIPPET}
            />
            <div className="hidden items-center justify-center border-x border-brand-400/20 bg-brand-500/10 lg:flex">
              <div className="flex h-11 w-11 items-center justify-center rounded-full bg-brand-500 shadow-[0_0_26px_rgba(66,68,147,0.55)]">
                <ArrowRight className="h-5 w-5 text-white" />
              </div>
            </div>
            <CodePanel
              label="Go"
              file="server.go"
              tone="cyan"
              code={GO_SNIPPET}
            />
          </div>
        </section>

        <section
          id="benchmarks"
          className="mx-auto max-w-[1480px] px-5 py-20 sm:px-8 lg:px-12 lg:py-24"
        >
          <div className="grid gap-8 lg:grid-cols-2 lg:items-end">
            <div>
              <SectionLabel>02 / Receipts</SectionLabel>
              <h2 className="text-balance font-syne text-[clamp(2.2rem,4vw,3.5rem)] font-extrabold leading-[1.02] tracking-[-0.05em] text-white">
                Numbers that survive a flame graph.
              </h2>
            </div>
            <p className="max-w-2xl text-[15px] leading-7 text-white/60 lg:ml-auto">
              Same workload, a JSON-emitting HTTP server with one DB query,
              compiled three ways. Results from repeated runs on Hetzner CCX23.
              Reproduce them yourself from the benchmark repo.
            </p>
          </div>

          <div className="mt-10 overflow-hidden rounded-[24px] border border-white/10 bg-panel">
            <div className="hidden grid-cols-[2fr_1fr_1fr_1.2fr_0.8fr] gap-4 border-b border-white/10 bg-brand-500/10 px-7 py-4 font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-200 md:grid">
              <div>Metric</div>
              <div>Node 20</div>
              <div>Bun 1.1</div>
              <div>Gun {"->"} Go</div>
              <div className="text-right">Speedup</div>
            </div>
            <div className="divide-y divide-white/10">
              {BENCHMARK_ROWS.map((row) => (
                <div
                  key={row.label}
                  className="grid gap-4 px-5 py-5 md:grid-cols-[2fr_1fr_1fr_1.2fr_0.8fr] md:px-7 md:py-6"
                >
                  <div className="font-medium text-white">{row.label}</div>
                  <MetricCell label="Node 20" value={row.node} />
                  <MetricCell label="Bun 1.1" value={row.bun} accent="muted" />
                  <MetricCell
                    label="Gun -> Go"
                    value={row.gun}
                    accent="brand"
                  />
                  <div className="font-syne text-2xl font-extrabold tracking-[-0.04em] text-green-400 md:text-right">
                    {row.speedup}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section
          id="examples"
          className="mx-auto max-w-[1480px] px-5 py-20 sm:px-8 lg:px-12 lg:py-24"
        >
          <div className="max-w-3xl">
            <SectionLabel>03 / Anatomy</SectionLabel>
            <h2 className="text-balance font-syne text-[clamp(2.2rem,4vw,3.5rem)] font-extrabold leading-[1.02] tracking-[-0.05em] text-white">
              Five stages between <span className="text-amber-300">.js</span>{" "}
              and <span className="text-cyan-300">./bin</span>.
            </h2>
            <p className="mt-5 text-[16px] leading-8 text-white/60">
              Gun is not a VM, not a wrapper, and not a bundler. It is a
              source-to-source compiler with a runtime designed so the emitted
              Go stays inspectable and operationally boring.
            </p>
          </div>

          <div className="mt-10 grid overflow-hidden rounded-[24px] border border-white/10 bg-panel lg:grid-cols-5">
            {STAGES.map((stage, index) => (
              <div
                key={stage.num}
                className="relative border-b border-white/10 px-6 py-7 last:border-b-0 lg:border-b-0 lg:border-r lg:last:border-r-0"
              >
                <div className="font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-brand-300">
                  {stage.num}
                </div>
                <div className="mt-5 inline-flex h-10 w-10 items-center justify-center rounded-xl border border-brand-300/35 bg-brand-500/15 text-sm font-bold text-brand-100">
                  {index + 1}
                </div>
                <h3 className="mt-5 font-syne text-2xl font-extrabold tracking-[-0.04em] text-white">
                  {stage.title}
                </h3>
                <p className="mt-3 text-sm leading-7 text-white/60">
                  {stage.description}
                </p>
              </div>
            ))}
          </div>
        </section>

        <section className="mx-auto max-w-[1480px] px-5 py-20 sm:px-8 lg:px-12 lg:py-24">
          <SectionLabel>04 / What you get</SectionLabel>
          <h2 className="max-w-2xl font-syne text-[clamp(2.2rem,4vw,3.5rem)] font-extrabold leading-[1.02] tracking-[-0.05em] text-white">
            Built for the parts that hurt.
          </h2>
          <div className="mt-10 grid gap-4 lg:grid-cols-6">
            {PILLARS.map((pillar, index) => (
              <div
                key={pillar.title}
                className={
                  index < 2
                    ? "rounded-[24px] border border-white/10 bg-panel p-7 lg:col-span-3"
                    : "rounded-[24px] border border-white/10 bg-panel p-7 lg:col-span-2"
                }
              >
                <h3 className="font-syne text-[1.55rem] font-extrabold leading-[1.08] tracking-[-0.04em] text-white">
                  {pillar.title}
                </h3>
                <p className="mt-4 text-[15px] leading-7 text-white/60">
                  {pillar.description}
                </p>
                {pillar.highlight ? (
                  <div className="mt-6 rounded-2xl border border-brand-400/20 bg-brand-500/10 p-4 font-mono text-[12px] leading-6 text-white/70">
                    {pillar.highlight.map((line) => (
                      <div key={line}>{line}</div>
                    ))}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </section>

        <section className="mx-auto max-w-[1480px] px-5 py-12 sm:px-8 lg:px-12 lg:py-16">
          <div className="grid gap-8 rounded-[28px] border border-brand-300/35 bg-[linear-gradient(135deg,rgba(66,68,147,0.18)_0%,rgba(66,68,147,0.04)_100%)] px-6 py-8 lg:grid-cols-[auto_1fr_auto] lg:items-center lg:px-12 lg:py-12">
            <div className="font-syne text-7xl font-extrabold leading-none text-brand-300/45">
              &ldquo;
            </div>
            <div>
              <p className="font-syne text-[clamp(1.4rem,3vw,2rem)] font-bold leading-[1.25] tracking-[-0.04em] text-white">
                We replaced a Node fleet with Gun-compiled binaries over a
                weekend. P99 fell 4x, our memory bill fell 3x, and nobody had to
                learn Go.
              </p>
              <div className="mt-6 flex items-center gap-4">
                <div className="flex h-11 w-11 items-center justify-center rounded-full bg-[linear-gradient(135deg,#424493,#666bd7)] text-sm font-bold text-white">
                  SR
                </div>
                <div>
                  <div className="text-sm font-semibold text-white">
                    Sara Rahimi
                  </div>
                  <div className="font-mono text-[12px] text-white/50">
                    Staff Engineer / platform team
                  </div>
                </div>
              </div>
            </div>
            <div className="flex justify-center lg:justify-end">
              <Mascot size={120} compact />
            </div>
          </div>
        </section>

        <section className="mx-auto grid max-w-[1480px] gap-14 px-5 py-20 sm:px-8 lg:grid-cols-[0.95fr_1.45fr] lg:px-12 lg:py-24">
          <div>
            <SectionLabel>05 / Questions</SectionLabel>
            <h2 className="text-balance font-syne text-[clamp(2.2rem,4vw,3.5rem)] font-extrabold leading-[1.02] tracking-[-0.05em] text-white">
              Things engineers ask before they install.
            </h2>
            <p className="mt-5 max-w-xl text-[15px] leading-7 text-white/60">
              Honest answers. If yours is not here, ask in Discord or open an
              issue and treat the generated Go as part of the contract.
            </p>
          </div>
          <div className="border-t border-white/10">
            {FAQS.map((item, index) => (
              <FAQItem
                key={item.question}
                index={index}
                question={item.question}
                answer={item.answer}
                open={openFaq === index}
                onToggle={() => setOpenFaq(openFaq === index ? -1 : index)}
              />
            ))}
          </div>
        </section>

        <section className="mx-auto max-w-[1480px] px-5 pb-24 sm:px-8 lg:px-12 lg:pb-30">
          <div className="relative overflow-hidden rounded-[30px] border border-brand-300/40 bg-[radial-gradient(ellipse_80%_100%_at_50%_100%,rgba(66,68,147,0.4)_0%,rgba(8,8,15,0.95)_70%)] px-6 py-10 lg:px-14 lg:py-16">
            <div className="absolute inset-0 bg-[linear-gradient(rgba(66,68,147,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(66,68,147,0.08)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_60%_80%_at_50%_100%,#000,transparent)]" />
            <div className="relative grid gap-10 lg:grid-cols-[1.35fr_1fr] lg:items-center">
              <div>
                <SectionLabel>Ready When You Are</SectionLabel>
                <h2 className="font-syne text-[clamp(2.5rem,6vw,4.5rem)] font-extrabold leading-[0.98] tracking-[-0.06em] text-white">
                  Pull the trigger.
                </h2>
                <p className="mt-5 max-w-2xl text-lg leading-8 text-white/60">
                  Free, MIT-licensed, and two commands away from your existing
                  package.json. The rewrite is the compiler, not your
                  application.
                </p>
                <div className="mt-8 flex flex-col gap-4 xl:flex-row xl:items-center">
                  <InstallTabs size="lg" />
                  <a
                    href="/docs"
                    className="inline-flex items-center justify-center gap-2 rounded-xl border border-brand-300/40 px-5 py-4 text-sm font-semibold text-white transition hover:bg-brand-500/10 xl:justify-start"
                  >
                    Read the docs
                    <ArrowRight className="h-4 w-4" />
                  </a>
                </div>
              </div>
              <div className="flex justify-center lg:justify-end">
                <Mascot size={220} />
              </div>
            </div>
          </div>
        </section>
      </main>

      <SiteFooter />
    </div>
  );
}

function CodePanel({
  label,
  file,
  tone,
  code,
}: {
  label: string;
  file: string;
  tone: "amber" | "cyan";
  code: string;
}) {
  const [highlighted, setHighlighted] = useState<string>("");

  useEffect(() => {
    let cancelled = false;

    highlightDemoCode(code, tone === "amber" ? "js" : "go").then((html) => {
      if (!cancelled) setHighlighted(html);
    });

    return () => {
      cancelled = true;
    };
  }, [code, tone]);

  return (
    <div className="min-w-0">
      <div className="flex items-center gap-3 border-b border-white/10 bg-white/5 px-5 py-4">
        <span
          className={
            tone === "amber"
              ? "rounded-md border border-amber-400/35 bg-amber-500/10 px-2 py-1 font-mono text-[10px] font-bold uppercase tracking-[0.12em] text-amber-300"
              : "rounded-md border border-cyan-400/35 bg-cyan-500/10 px-2 py-1 font-mono text-[10px] font-bold uppercase tracking-[0.12em] text-cyan-300"
          }
        >
          {label}
        </span>
        <span className="font-mono text-[12px] text-white/35">{file}</span>
      </div>
      {highlighted ? (
        <div
          className="overflow-x-auto px-3 py-6 text-[12px] sm:text-[13px] [&_.shiki-demo]:m-0 [&_.shiki-demo]:bg-transparent [&_.shiki-demo]:p-0 [&_.shiki-demo]:font-mono [&_.shiki-demo]:text-white/90 [&_.shiki-demo]:text-[12px] sm:[&_.shiki-demo]:text-[13px] [&_.shiki-demo__code]:grid [&_.line]:block [&_.line]:leading-[0]"
          dangerouslySetInnerHTML={{ __html: highlighted }}
        />
      ) : (
        <pre className="overflow-x-auto px-3 py-6 font-mono text-[12px] text-white/90 sm:text-[13px]">
          <code>{code}</code>
        </pre>
      )}
    </div>
  );
}

function MetricCell({
  label,
  value,
  accent = "base",
}: {
  label: string;
  value: string;
  accent?: "base" | "muted" | "brand";
}) {
  const bar =
    accent === "brand"
      ? "bg-brand-400"
      : accent === "muted"
        ? "bg-brand-300/70"
        : "bg-white/25";

  return (
    <div>
      <div className="mb-2 font-mono text-[10px] font-bold uppercase tracking-[0.12em] text-white/35 md:hidden">
        {label}
      </div>
      <div className="mb-2 h-1.5 rounded-full bg-white/5">
        <div className={`${bar} h-full rounded-full`} />
      </div>
      <div
        className={
          accent === "brand"
            ? "font-mono text-[13px] font-semibold text-white"
            : "font-mono text-[13px] text-white/60"
        }
      >
        {value}
      </div>
    </div>
  );
}
