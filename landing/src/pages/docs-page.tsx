import { Search } from 'lucide-react'
import { startTransition, useDeferredValue, useMemo, useState, type ReactNode } from 'react'
import { CodeBlock, InlineCode, Note, SiteFooter, SiteHeader } from '../components/site'

type DocPage = {
  lead?: string
  body: ReactNode
  sections: string[]
}

const SIDEBAR = [
  { label: 'Getting Started', icon: '01', items: ['Introduction', 'Installation', 'Quick Start'] },
  { label: 'Core Concepts', icon: '02', items: ['How It Works', 'JSValue Runtime', 'Event Loop'] },
  { label: 'Language', icon: '03', items: ['Variables', 'Functions', 'Classes', 'Async / Await', 'Modules'] },
  { label: 'APIs', icon: '04', items: ['Node.js Compat', 'Bun Compat', 'npm Dependencies'] },
  { label: 'Configuration', icon: '05', items: ['gun.config.js', 'CLI Reference', 'Source Maps'] },
  { label: 'Advanced', icon: '06', items: ['Incremental Builds', 'CI Integration', 'Debugging'] },
] as const

export const ALL_PAGES = SIDEBAR.flatMap((group) => group.items)

export const DOC_PAGES: Record<string, DocPage> = {
  Introduction: {
    lead:
      'Gun is a JavaScript-to-Go transpiler. It converts your JS or TS source and its npm dependencies into valid Go code that runs natively without Node.js in production.',
    sections: ['Why Gun?', 'When not to use Gun'],
    body: (
      <>
        <DocParagraph>
          Unlike source-to-source translators that try to produce idiomatic Go, Gun preserves JavaScript semantics through a lightweight runtime called <InlineCode>jsvalue</InlineCode>. Dynamic typing, prototype chains, and the event loop stay intact.
        </DocParagraph>
        <Note type="tip">
          Gun is not trying to turn your JS into hand-written Go. It is trying to make your JS run as Go, compiled and deployment-friendly.
        </Note>
        <DocSection title="Why Gun?">
          <DocParagraph>
            JavaScript is fast to write. Go is fast to run. Gun gives you both without the rewrite tax of porting a production codebase by hand.
          </DocParagraph>
          <div className="grid gap-4 sm:grid-cols-2">
            {[
              ['10x faster runtime', 'Go compiled runtime versus Node.js V8.'],
              ['Zero runtime deps', 'No Node.js or Bun required on servers.'],
              ['Node and Bun APIs', 'Common built-ins stay available.'],
              ['npm deps included', 'Dependencies are transpiled with your app.'],
            ].map(([title, description]) => (
              <div key={title} className="rounded-2xl border border-white/10 bg-brand-500/10 px-5 py-4">
                <div className="text-sm font-semibold text-white">{title}</div>
                <div className="mt-1 text-sm leading-6 text-white/60">{description}</div>
              </div>
            ))}
          </div>
        </DocSection>
        <DocSection title="When not to use Gun">
          <DocParagraph>
            Gun is best for server-side JS that wants to ship as a static Go binary. It is not a front-end bundler, and it is not the right fit for runtime <InlineCode>eval</InlineCode> heavy workloads or V8-specific behavior you cannot model statically.
          </DocParagraph>
        </DocSection>
      </>
    ),
  },
  Installation: {
    lead: 'Gun ships as an npm package. You need Node.js 18+ or Bun 1.x for the CLI and Go 1.21+ to compile the output.',
    sections: ['Prerequisites', 'Install the CLI', 'Verify installation', 'Go runtime module'],
    body: (
      <>
        <DocSection title="Prerequisites">
          <CodeBlock lang="bash" code={`node --version    # -> v20.x or higher\ngo version        # -> 1.21+`} />
        </DocSection>
        <DocSection title="Install the CLI">
          <DocParagraph>Pick your package manager. The CLI is pure JavaScript and does not ship as a native binary.</DocParagraph>
          <CodeBlock lang="bash" code={`npm i -g gun-transpiler\n# or\nbun add -g gun-transpiler`} />
        </DocSection>
        <DocSection title="Verify installation">
          <CodeBlock lang="bash" code={`gun --version\n# -> gun v1.0.2`} />
          <Note>
            Gun also works as a local dependency. Run it with <InlineCode>npx gun</InlineCode> or through package scripts if you prefer a locked CLI version.
          </Note>
        </DocSection>
        <DocSection title="Go runtime module">
          <DocParagraph>The transpiled output imports Gun&apos;s Go runtime, so you should add it to your Go module up front.</DocParagraph>
          <CodeBlock lang="bash" code={`go get github.com/nnstd/gun/runtime`} />
        </DocSection>
      </>
    ),
  },
  'Quick Start': {
    lead: 'Transpile your first file in under a minute.',
    sections: ['Write some JavaScript', 'Transpile', 'Run', 'Build a binary'],
    body: (
      <>
        <DocSection title="Write some JavaScript">
          <CodeBlock
            lang="js"
            code={`import { createServer } from 'http'\n\nconst port = 8080\n\ncreateServer((req, res) => {\n  res.writeHead(200)\n  res.end('Hello from Go!\\n')\n}).listen(port)\n\nconsole.log(\`Listening on :\${port}\`)`}
          />
        </DocSection>
        <DocSection title="Transpile">
          <CodeBlock lang="bash" code={`gun transpile server.js -o server.go`} />
        </DocSection>
        <DocSection title="Run">
          <CodeBlock lang="bash" code={`go run server.go\n# -> Listening on :8080`} />
          <Note type="tip">
            Use <InlineCode>gun watch</InlineCode> during development to auto-transpile on file changes.
          </Note>
        </DocSection>
        <DocSection title="Build a binary">
          <CodeBlock lang="bash" code={`go build -o server ./...\n./server`} />
        </DocSection>
      </>
    ),
  },
  'How It Works': {
    lead: 'Gun runs a compiler pipeline with parse, analysis, emission, and linking phases instead of embedding a JS runtime in production.',
    sections: ['Compiler stages'],
    body: (
      <DocSection title="Compiler stages">
        <div className="space-y-5">
          {[
            ['01', 'Parse', 'Gun walks your JS or TS source using Tree-sitter and builds a concrete syntax tree for every file, including npm dependencies.'],
            ['02', 'Analyze', 'A type-flow pass resolves variable scopes, infers types from usage, and maps JS constructs to Go equivalents.'],
            ['03', 'Emit', 'The Go emitter outputs valid Go code while values flow through the JSValue runtime to preserve JavaScript semantics.'],
            ['04', 'Link', 'Import paths are rewritten, go.mod is updated, and the output is formatted with gofmt.'],
          ].map(([num, title, description]) => (
            <div key={num} className="flex gap-4 rounded-2xl border border-white/10 bg-panel px-5 py-5">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-brand-300/35 bg-brand-500/15 font-mono text-[11px] font-bold uppercase tracking-[0.12em] text-brand-200">
                {num}
              </div>
              <div>
                <div className="text-sm font-semibold text-white">{title}</div>
                <div className="mt-1 text-sm leading-7 text-white/60">{description}</div>
              </div>
            </div>
          ))}
        </div>
      </DocSection>
    ),
  },
  'JSValue Runtime': {
    lead: 'Every value in transpiled code becomes a JSValue so the generated Go can preserve JavaScript behavior precisely.',
    sections: ['Constructors', 'Property access'],
    body: (
      <>
        <DocSection title="Constructors">
          <CodeBlock lang="go" code={`jsvalue.NewNumber(float64(42))\njsvalue.NewString("hello")\njsvalue.NewBool(true)\njsvalue.NewFunction(myFunc)\njsvalue.ObjectFrom(map[string]any{"key": val})`} />
        </DocSection>
        <DocSection title="Property access">
          <CodeBlock lang="go" code={`obj.Get("key")             // obj.key\nobj.Set("key", val)        // obj.key = val\nobj.MethodCall("fn", arg)  // obj.fn(arg)\nfn.Call(arg1, arg2)        // fn(arg1, arg2)`} />
          <Note>
            You normally do not call the runtime directly. Gun emits these helpers for you when it lowers JavaScript operations into Go.
          </Note>
        </DocSection>
      </>
    ),
  },
  'Event Loop': {
    lead: 'Gun includes an event loop runtime that mirrors async semantics from Node.js and Bun.',
    sections: ['Runtime entrypoint'],
    body: (
      <DocSection title="Runtime entrypoint">
        <CodeBlock lang="go" code={`import eventloop "github.com/nnstd/gun/runtime/eventloop"\n\nfunc main() {\n    defer error.RecoverMain()\n    // ... transpiled code ...\n    eventloop.Default.Run()\n}`} />
      </DocSection>
    ),
  },
  Variables: stubPage('Variables'),
  Functions: stubPage('Functions'),
  Classes: stubPage('Classes'),
  'Async / Await': stubPage('Async / Await'),
  Modules: stubPage('Modules'),
  'Node.js Compat': stubPage('Node.js Compat'),
  'Bun Compat': stubPage('Bun Compat'),
  'npm Dependencies': {
    lead: 'Gun transpiles your dependencies along with your source so the final Go binary stays self-contained.',
    sections: ['Why this matters', 'Handling edge cases'],
    body: (
      <>
        <DocSection title="Why this matters">
          <DocParagraph>
            If Gun only transpiled your own code, the output would still need a JavaScript runtime for the rest of the graph. Full dependency transpilation is what lets the final binary stand alone.
          </DocParagraph>
          <CodeBlock lang="bash" code={`gun transpile src/index.js -o go/`} />
        </DocSection>
        <DocSection title="Handling edge cases">
          <DocParagraph>
            Packages with native addons can be mapped to hand-written Go equivalents through <InlineCode>gun.config.js</InlineCode> aliases.
          </DocParagraph>
          <CodeBlock lang="js" code={`export default {\n  aliases: {\n    bcrypt: 'github.com/my/bcrypt-go',\n  },\n}`} />
        </DocSection>
      </>
    ),
  },
  'gun.config.js': {
    lead: 'Place a gun.config.js at your project root to control entrypoints, output, source maps, and dependency aliases.',
    sections: ['Example config', 'Options'],
    body: (
      <>
        <DocSection title="Example config">
          <CodeBlock lang="js" code={`export default {\n  entry: 'src/index.js',\n  out: 'go/',\n  sourceMaps: true,\n  module: 'github.com/my/app',\n  aliases: {\n    lodash: 'github.com/my/lodash-go',\n  },\n}`} />
        </DocSection>
        <DocSection title="Options">
          <div className="overflow-hidden rounded-2xl border border-white/10 bg-panel">
            {[
              ['entry', 'string', 'Entry point file'],
              ['out', 'string', 'Output directory'],
              ['sourceMaps', 'boolean', 'Emit source maps'],
              ['module', 'string', 'Go module path'],
              ['aliases', 'object', 'npm package to Go module overrides'],
            ].map(([name, type, description]) => (
              <div key={name} className="grid gap-3 border-b border-white/10 px-5 py-4 last:border-b-0 md:grid-cols-[140px_100px_1fr]">
                <InlineCode>{name}</InlineCode>
                <span className="font-mono text-[12px] text-green-300">{type}</span>
                <span className="text-sm leading-7 text-white/60">{description}</span>
              </div>
            ))}
          </div>
        </DocSection>
      </>
    ),
  },
  'CLI Reference': {
    lead: 'Gun ships a small CLI surface focused on transpilation, watch mode, checking, and pipeline debugging.',
    sections: ['gun transpile', 'gun watch', 'gun check', 'gun debug'],
    body: (
      <>
        <DocSection title="gun transpile">
          <CodeBlock lang="bash" code={`gun transpile <input> [flags]\n\nFlags:\n  -o, --out <path>    Output file or directory\n  -w, --watch         Watch for changes\n  --source-maps       Emit source maps\n  --config <path>     Path to gun.config.js`} />
        </DocSection>
        <DocSection title="gun watch">
          <CodeBlock lang="bash" code={`gun watch src/ -o go/\n# -> Watching 142 files\n# -> Changed: src/server.js (re-transpiled in 12ms)`} />
        </DocSection>
        <DocSection title="gun check">
          <CodeBlock lang="bash" code={`gun check src/\n# -> ✓ 142 files OK`} />
        </DocSection>
        <DocSection title="gun debug">
          <CodeBlock lang="bash" code={`gun debug src/server.js --stage=analyze`} />
        </DocSection>
      </>
    ),
  },
  'Source Maps': {
    lead: 'Pass --source-maps and Gun emits a .go.map alongside each Go file so production traces map back to the source.',
    sections: ['Emit source maps'],
    body: (
      <DocSection title="Emit source maps">
        <CodeBlock lang="bash" code={`gun transpile src/ -o go/ --source-maps`} />
      </DocSection>
    ),
  },
  'Incremental Builds': {
    lead: 'Gun watch mode reuses Tree-sitter incremental parsing so only the modified subtree is reanalyzed and re-emitted.',
    sections: ['Performance profile'],
    body: (
      <DocSection title="Performance profile">
        <DocParagraph>
          Cold builds of a 50k-line codebase typically finish in under four seconds. Warm rebuilds after a single-file edit usually land under 20ms.
        </DocParagraph>
      </DocSection>
    ),
  },
  'CI Integration': {
    lead: 'A typical CI flow runs gun check on pull requests and gun transpile plus go build on release or preview branches.',
    sections: ['Example workflow'],
    body: (
      <DocSection title="Example workflow">
        <CodeBlock lang="bash" code={`# .github/workflows/ci.yml\n- run: npm i -g gun-transpiler\n- run: gun check src/\n- run: gun transpile src/ -o go/\n- run: go build -o bin/server ./go/...`} />
      </DocSection>
    ),
  },
  Debugging: {
    lead: 'Source maps plus go run or delve give you a JS-oriented debugging experience on top of a Go binary.',
    sections: ['Step through transpiled code'],
    body: (
      <DocSection title="Step through transpiled code">
        <CodeBlock lang="bash" code={`dlv debug ./go/server.go\n\n# View the original JS source for any frame\n(dlv) source-map`} />
        <Note type="warn">
          If you hit a runtime error that is hard to trace, run with <InlineCode>GUN_TRACE=1</InlineCode> so each JSValue operation logs its source location.
        </Note>
      </DocSection>
    ),
  },
}

export function DocsPage() {
  const [active, setActive] = useState('Introduction')
  const [search, setSearch] = useState('')
  const deferredSearch = useDeferredValue(search)

  const page = DOC_PAGES[active] ?? DOC_PAGES.Introduction
  const activeIndex = ALL_PAGES.indexOf(active)

  const filteredSidebar = useMemo(() => {
    const query = deferredSearch.trim().toLowerCase()
    if (!query) return SIDEBAR

    return SIDEBAR.map((group) => ({
      ...group,
      items: group.items.filter((item) => item.toLowerCase().includes(query)),
    })).filter((group) => group.items.length > 0)
  }, [deferredSearch])

  function setActivePage(next: string) {
    startTransition(() => setActive(next))
  }

  function groupOf(item: string) {
    return SIDEBAR.find((group) => group.items.some((entry) => entry === item))?.label ?? 'Docs'
  }

  return (
    <div className="min-h-screen">
      <SiteHeader current="docs" crumb={`Docs / ${groupOf(active)}`} />

      <main className="mx-auto grid max-w-[1480px] lg:grid-cols-[280px_minmax(0,1fr)_220px]">
        <aside className="border-b border-white/10 px-5 py-8 lg:sticky lg:top-[89px] lg:h-[calc(100vh-89px)] lg:overflow-y-auto lg:border-b-0 lg:border-r lg:px-0">
          <div className="px-0 lg:px-5">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-brand-300" />
              <input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Search docs..."
                className="w-full rounded-xl border border-white/10 bg-brand-500/10 px-10 py-3 text-sm text-white outline-none transition placeholder:text-white/35 focus:border-brand-300/45"
              />
            </div>
          </div>

          <div className="mt-6 space-y-6">
            {filteredSidebar.length === 0 ? <div className="px-2 text-sm text-white/40 lg:px-5">No results.</div> : null}
            {filteredSidebar.map((group) => (
              <div key={group.label}>
                <div className="mb-2 flex items-center gap-3 px-2 lg:px-5">
                  <span className="rounded-md border border-white/10 bg-brand-500/10 px-1.5 py-1 font-mono text-[9px] font-bold uppercase tracking-[0.12em] text-brand-200">
                    {group.icon}
                  </span>
                  <span className="font-mono text-[11px] font-bold uppercase tracking-[0.16em] text-brand-300">
                    {group.label}
                  </span>
                </div>
                <div>
                  {group.items.map((item) => {
                    const isActive = item === active
                    return (
                      <button
                        key={item}
                        type="button"
                        onClick={() => setActivePage(item)}
                        className={isActive ? 'block w-full border-l-2 border-brand-400 bg-brand-500/15 px-4 py-2 text-left text-[14px] font-semibold text-white transition lg:px-5' : 'block w-full border-l-2 border-transparent px-4 py-2 text-left text-[14px] text-white/60 transition hover:bg-brand-500/8 hover:text-white lg:px-5'}
                      >
                        {item}
                      </button>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>

          <div className="mt-8 rounded-2xl border border-white/10 bg-brand-500/10 p-4 lg:mx-5">
            <div className="font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-green-300">Edge release</div>
            <p className="mt-2 text-sm leading-6 text-white/60">
              v1.1 preview ships next week with faster incremental rebuilds and full Bun parity.
            </p>
          </div>
        </aside>

        <section className="min-w-0 px-5 py-10 sm:px-8 lg:px-16 lg:py-14">
          <div className="max-w-3xl">
            <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-brand-400/25 bg-brand-500/10 px-3 py-1.5 font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-200">
              <span className="rounded-full bg-brand-500 px-2 py-1 text-white">v1.0.2</span>
              Latest / April 2026
            </div>
            <h1 className="font-syne text-[clamp(2.4rem,5vw,3.5rem)] font-extrabold leading-[1.02] tracking-[-0.05em] text-white">
              {active}
            </h1>
            {page.lead ? <p className="mt-5 text-lg leading-8 text-white/60">{page.lead}</p> : null}
            <div className="mt-10 space-y-8">{page.body}</div>

            <div className="mt-14 grid gap-4 border-t border-white/10 pt-8 md:grid-cols-2">
              {activeIndex > 0 ? (
                <PagerButton direction="prev" label={ALL_PAGES[activeIndex - 1]} onClick={() => setActivePage(ALL_PAGES[activeIndex - 1])} />
              ) : (
                <div />
              )}
              {activeIndex < ALL_PAGES.length - 1 ? (
                <PagerButton direction="next" label={ALL_PAGES[activeIndex + 1]} onClick={() => setActivePage(ALL_PAGES[activeIndex + 1])} />
              ) : (
                <div />
              )}
            </div>
          </div>
        </section>

        <aside className="hidden border-l border-white/10 px-6 py-14 lg:block">
          <div className="sticky top-[110px]">
            <div className="font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-300">On this page</div>
            <div className="mt-4 border-l border-white/10 pl-4">
              <div className="text-sm font-medium text-white">{active}</div>
              <div className="mt-3 space-y-2">
                {page.sections.map((section) => (
                  <div key={section} className="text-sm text-white/50">
                    {section}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </aside>
      </main>

      <SiteFooter />
    </div>
  )
}

function stubPage(title: string): DocPage {
  return {
    lead: 'This section is being rewritten from the latest landing HTML and will expand as the compiler surface settles.',
    sections: ['Status'],
    body: (
      <DocSection title="Status">
        <DocParagraph>{title} documentation is not filled out yet, but the page shell and navigation are now aligned with the refreshed docs variant.</DocParagraph>
      </DocSection>
    ),
  }
}

function DocParagraph({ children }: { children: ReactNode }) {
  return <p className="text-[15px] leading-8 text-white/65">{children}</p>
}

function DocSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2 className="mb-4 border-t border-white/10 pt-8 font-syne text-[1.6rem] font-extrabold tracking-[-0.04em] text-white">
        {title}
      </h2>
      <div className="space-y-5">{children}</div>
    </section>
  )
}

function PagerButton({ direction, label, onClick }: { direction: 'prev' | 'next'; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={direction === 'next' ? 'rounded-2xl border border-white/10 bg-brand-500/8 px-5 py-4 text-right transition hover:border-brand-300/35 hover:bg-brand-500/12' : 'rounded-2xl border border-white/10 bg-brand-500/8 px-5 py-4 text-left transition hover:border-brand-300/35 hover:bg-brand-500/12'}
    >
      <div className="font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-300">
        {direction === 'prev' ? '<- Previous' : 'Next ->'}
      </div>
      <div className="mt-1 text-sm font-semibold text-white">{label}</div>
    </button>
  )
}
