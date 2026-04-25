import { HeadContent, Link, Scripts, createRootRoute } from '@tanstack/react-router'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'
import { TanStackDevtools } from '@tanstack/react-devtools'
import { AlertTriangle, ArrowLeft, ArrowRight, Home, RefreshCw } from 'lucide-react'

import appCss from '../styles.css?url'
import { Mascot, MascotMark, SiteFooter } from '../components/site'

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'Gun — Compile JavaScript to Go' },
      {
        name: 'description',
        content:
          'Gun transpiles your JavaScript codebase and its npm dependencies into Go, with Bun and Node compatibility and a landing, docs, and blog experience rebuilt from the latest HTML variants.',
      },
    ],
    links: [
      { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
      { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossOrigin: 'anonymous' as const },
      {
        rel: 'stylesheet',
        href: 'https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@300;400;500;600;700&family=JetBrains+Mono:wght@300;400;500;700&family=Syne:wght@700;800&display=swap',
      },
      { rel: 'stylesheet', href: appCss },
    ],
  }),
  notFoundComponent: NotFoundPage,
  errorComponent: ErrorPage,
  shellComponent: RootDocument,
})

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body>
        <div id="app">{children}</div>
        <TanStackDevtools
          config={{ position: 'bottom-right' }}
          plugins={[{ name: 'Tanstack Router', render: <TanStackRouterDevtoolsPanel /> }]}
        />
        <Scripts />
      </body>
    </html>
  )
}

function NotFoundPage() {
  const attemptedPath = typeof window === 'undefined' ? '/docs/missing-page' : `${window.location.pathname}${window.location.search}`

  return (
    <ErrorShell
      eyebrow="ERR_PATH_NOT_RESOLVED"
      eyebrowAccent="amber"
      code="404"
      title="This route never made it through the transpiler."
      description="Gun walked the dependency graph and came back empty-handed. The page you tried to open does not exist, or it moved while no one was watching."
      panel={
        <div className="rounded-2xl border border-white/10 bg-black/45 p-5 font-mono text-[13px] leading-7 text-white/70 shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div>
            <span className="text-brand-300">$</span>{' '}
            <span className="text-white">gun resolve </span>
            <span className="text-white/35">{attemptedPath}</span>
          </div>
          <div className="text-white/35">&gt; walking module graph...</div>
          <div>
            <span className="text-amber-300">✗ unresolved import</span>{' '}
            <span className="text-white/35">no source file found</span>
          </div>
          <div>
            <span className="text-green-300">-&gt;</span>{' '}
            <span className="text-white/35">try one of the suggestions below</span>
          </div>
        </div>
      }
      actions={
        <>
          <Link
            to="/"
            className="inline-flex items-center gap-2 rounded-xl bg-brand-500 px-5 py-3 text-sm font-semibold text-white shadow-[0_0_24px_rgba(66,68,147,0.45)] transition hover:bg-brand-400"
          >
            <Home className="h-4 w-4" />
            Take me home
          </Link>
          <Link
            to="/docs"
            className="inline-flex items-center gap-2 rounded-xl border border-brand-300/35 px-5 py-3 text-sm font-semibold text-white transition hover:bg-brand-500/10"
          >
            Read the docs
            <ArrowRight className="h-4 w-4" />
          </Link>
        </>
      }
      supplemental={
        <div className="border-t border-white/10 pt-6">
          <div className="font-mono text-[10px] font-bold uppercase tracking-[0.14em] text-brand-300">Probably what you wanted</div>
          <div className="mt-4 flex flex-col gap-3 text-sm text-white/60">
            <Link to="/" hash="playground" className="inline-flex items-center gap-2 transition hover:text-white">
              <ArrowRight className="h-4 w-4 text-brand-300" />
              Playground
            </Link>
            <Link to="/" hash="benchmarks" className="inline-flex items-center gap-2 transition hover:text-white">
              <ArrowRight className="h-4 w-4 text-brand-300" />
              Benchmarks
            </Link>
            <Link to="/docs" className="inline-flex items-center gap-2 transition hover:text-white">
              <ArrowRight className="h-4 w-4 text-brand-300" />
              Documentation
            </Link>
            <Link to="/blog" className="inline-flex items-center gap-2 transition hover:text-white">
              <ArrowRight className="h-4 w-4 text-brand-300" />
              Blog
            </Link>
          </div>
        </div>
      }
      footerStatus="all systems normal; this page just is not one of them"
      mascot={
        <div className="relative flex items-center justify-center">
          <div className="absolute h-[280px] w-[280px] rounded-full bg-[radial-gradient(circle,rgba(66,68,147,0.35)_0%,transparent_70%)]" />
          <div className="relative">
            <Mascot size={240} />
            <div className="absolute -right-2 top-2 font-syne text-4xl font-extrabold text-brand-200/80">?</div>
            <div className="absolute left-0 top-8 font-syne text-2xl font-extrabold text-brand-200/45">?</div>
          </div>
        </div>
      }
    />
  )
}

function ErrorPage({ error, reset }: { error: Error; reset: () => void }) {
  const incidentId = `GUN-${Math.random().toString(36).slice(2, 8).toUpperCase()}`

  return (
    <ErrorShell
      eyebrow="ERR_RUNTIME_PANIC"
      eyebrowAccent="rose"
      code="500"
      title="Something panicked on our side."
      description="An uncaught error reached the top of the event loop. It is not your fault, and it is not yours to fix. We already have the trace and incident context."
      panel={
        <div className="overflow-hidden rounded-2xl border border-white/10 bg-black/45 font-mono text-[12px] leading-7 text-white/70 shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="flex items-center gap-3 border-b border-white/10 bg-rose-500/8 px-4 py-3 text-[11px] font-bold uppercase tracking-[0.12em] text-rose-300">
            <span className="h-2 w-2 rounded-full bg-rose-300 shadow-[0_0_10px_rgba(251,113,133,0.9)]" />
            panic - uncaught error
            <span className="ml-auto text-[10px] text-white/35">incident {incidentId}</span>
          </div>
          <div className="space-y-1 px-4 py-4">
            <div className="font-semibold text-rose-300">{error.message || 'JSValue.TypeError: unexpected runtime failure'}</div>
            <div className="border-l-2 border-rose-400/70 bg-rose-500/6 px-3 py-1 text-white/80">at handleRequest (runtime/http/server.go:142)</div>
            <div className="text-white/45">at eventloop.tick (runtime/eventloop/loop.go:88)</div>
            <div className="text-white/45">at main.serve (build/main.go:31)</div>
            <div className="text-white/45">at runtime.goexit (go/src/runtime/asm_amd64.s:1650)</div>
          </div>
        </div>
      }
      actions={
        <>
          <button
            type="button"
            onClick={reset}
            className="inline-flex items-center gap-2 rounded-xl bg-brand-500 px-5 py-3 text-sm font-semibold text-white shadow-[0_0_24px_rgba(66,68,147,0.45)] transition hover:bg-brand-400"
          >
            <RefreshCw className="h-4 w-4" />
            Try again
          </button>
          <Link
            to="/"
            className="inline-flex items-center gap-2 rounded-xl border border-brand-300/35 px-5 py-3 text-sm font-semibold text-white transition hover:bg-brand-500/10"
          >
            <ArrowLeft className="h-4 w-4" />
            Back to home
          </Link>
        </>
      }
      supplemental={
        <div className="border-t border-white/10 pt-6 text-sm leading-7 text-white/60">
          <div className="font-mono text-[10px] font-bold uppercase tracking-[0.14em] text-brand-300">If this keeps happening</div>
          <p className="mt-3">
            Check <span className="text-white">status.gun.dev</span> for live incidents or open an issue with incident ID{' '}
            <span className="font-mono text-white">{incidentId}</span>. For runtime failures in your own builds, start in the debugging guide.
          </p>
        </div>
      }
      footerStatus="service degraded; the binary is fine, this request was not"
      mascot={
        <div className="relative flex items-center justify-center">
          <div className="absolute h-[280px] w-[280px] rounded-full bg-[radial-gradient(circle,rgba(244,114,182,0.22)_0%,transparent_70%)]" />
          <div className="relative flex items-center justify-center rounded-full border border-rose-400/20 bg-rose-500/5 p-10">
            <AlertTriangle className="h-32 w-32 text-rose-300/85" strokeWidth={1.5} />
          </div>
        </div>
      }
    />
  )
}

function ErrorShell({
  eyebrow,
  eyebrowAccent,
  code,
  title,
  description,
  panel,
  actions,
  supplemental,
  footerStatus,
  mascot,
}: {
  eyebrow: string
  eyebrowAccent: 'amber' | 'rose'
  code: string
  title: string
  description: string
  panel: React.ReactNode
  actions: React.ReactNode
  supplemental: React.ReactNode
  footerStatus: string
  mascot: React.ReactNode
}) {
  const accentClass =
    eyebrowAccent === 'rose'
      ? 'border-rose-400/30 bg-rose-500/10 text-rose-300'
      : 'border-brand-300/30 bg-brand-500/10 text-brand-200'

  const dotClass = eyebrowAccent === 'rose' ? 'bg-rose-300' : 'bg-amber-300'

  return (
    <div className="min-h-screen">
      <header className="border-b border-white/10 bg-black/30 backdrop-blur-xl">
        <div className="mx-auto flex max-w-370 items-center justify-between px-5 py-[18px] sm:px-8 lg:px-12">
          <Link to="/" className="flex items-center gap-3">
            <MascotMark size={32} />
            <span className="font-syne text-[24px] font-extrabold leading-none tracking-[-0.5px] text-white">gun</span>
          </Link>
          <Link
            to="/"
            className="inline-flex items-center gap-2 rounded-[8px] border border-white/10 px-[14px] py-[8px] font-mono text-[12px] text-white/60 transition hover:border-brand-300/55 hover:bg-brand-500/8 hover:text-white"
          >
            <ArrowLeft className="h-4 w-4" />
            Back to gun
          </Link>
        </div>
      </header>

      <main className="mx-auto grid max-w-370 gap-12 px-5 py-14 sm:px-8 lg:grid-cols-[1.2fr_1fr] lg:items-center lg:px-12 lg:py-20">
        <section>
          <div className={`mb-7 inline-flex items-center gap-3 rounded-full border px-3 py-2 font-mono text-[11px] font-bold uppercase tracking-[0.16em] ${accentClass}`}>
            <span className={`h-2 w-2 rounded-full shadow-[0_0_10px_currentColor] ${dotClass}`} />
            {eyebrow}
          </div>
          <h1 className="font-syne text-[clamp(7rem,16vw,14rem)] font-extrabold leading-[0.82] tracking-[-0.06em] text-white">
            {code}
          </h1>
          <h2 className="mt-2 max-w-3xl font-syne text-[clamp(1.8rem,4vw,2.75rem)] font-extrabold leading-[1.02] tracking-tighter text-white">
            {title}
          </h2>
          <p className="mt-5 max-w-2xl text-[16px] leading-8 text-white/60">{description}</p>
          <div className="mt-8 max-w-3xl">{panel}</div>
          <div className="mt-8 flex flex-wrap gap-3">{actions}</div>
          <div className="mt-8 max-w-3xl">{supplemental}</div>
        </section>

        <aside>{mascot}</aside>
      </main>

      <SiteFooter />
      <div className="mx-auto max-w-370 px-5 pb-8 sm:px-8 lg:px-12">
        <div className="border-t border-white/10 pt-5 font-mono text-[12px] text-white/35">{footerStatus}</div>
      </div>
    </div>
  )
}
