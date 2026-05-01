import { Search } from 'lucide-react'
import { startTransition, useDeferredValue, useMemo, useState } from 'react'
import { allDocs } from '../../.content-collections/generated'
import { MarkdownContent } from '../components/markdown'
import { SiteFooter, SiteHeader } from '../components/site'

type DocPage = {
  lead?: string
  html: string
  sections: string[]
}

const SIDEBAR = [
  { label: 'Get Started', icon: '01', items: ['Introduction', 'Installation', 'Quick Start'] },
  { label: 'Core Runtime', icon: '02', items: ['How It Works', 'Runtime Semantics', 'Event Loop', 'Debugging'] },
  { label: 'Language', icon: '03', items: ['Variables', 'Functions', 'Classes', 'Async / Await', 'Modules'] },
  { label: 'HTTP & Networking', icon: '04', items: ['HTTP Server', 'Fetch', 'OpenTelemetry'] },
  { label: 'File & Module System', icon: '05', items: ['npm Dependencies', 'Source Maps'] },
  { label: 'Interop & Tooling', icon: '06', items: ['Node.js Compat', 'Bun Compat', 'FFI', 'C Compiler'] },
  { label: 'Configuration', icon: '07', items: ['Project Scripts', 'CLI Reference'] },
  { label: 'Advanced', icon: '08', items: ['Incremental Builds', 'CI Integration'] },
] as const

export const ALL_PAGES = SIDEBAR.flatMap((group) => group.items)

export const DOC_PAGES: Record<string, DocPage> = Object.fromEntries(
  allDocs.map((doc) => [
    doc.title,
    {
      lead: doc.lead,
      html: withSectionAnchors(doc.html, doc.sections),
      sections: doc.sections,
    },
  ]),
)

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
    startTransition(() => {
      setActive(next)
      window.location.hash = ''
    })
  }

  function groupOf(item: string) {
    return SIDEBAR.find((group) => group.items.some((entry) => entry === item))?.label ?? 'Docs'
  }

  return (
    <div className="min-h-screen">
      <SiteHeader current="docs" crumb={`Docs / ${groupOf(active)}`} />

      <main className="mx-auto grid max-w-370 lg:grid-cols-[280px_minmax(0,1fr)_220px]">
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
            <h1 className="font-syne text-[clamp(2.4rem,5vw,3.5rem)] font-extrabold leading-[1.02] tracking-tighter text-white">
              {active}
            </h1>
            {page.lead ? <p className="mt-5 text-lg leading-8 text-white/60">{page.lead}</p> : null}
            <div className="mt-10">
              <MarkdownContent html={page.html} />
            </div>

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
                  <a
                    key={section}
                    href={`#${slugifyHeading(section)}`}
                    className="block text-sm text-white/50 transition hover:text-white"
                  >
                    {section}
                  </a>
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

function slugifyHeading(value: string) {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function withSectionAnchors(html: string, sections: string[]) {
  return sections.reduce((result, section) => {
    const heading = `<h2>${section}</h2>`
    const anchoredHeading = `<h2 id="${slugifyHeading(section)}">${section}</h2>`

    return result.replace(heading, anchoredHeading)
  }, html)
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
