import { ArrowLeft, ArrowRight } from 'lucide-react'
import { startTransition, useMemo, useState } from 'react'
import { allBlogs } from 'content-collections'
import { MarkdownContent } from '../components/markdown'
import { SiteFooter, SiteHeader, Tag } from '../components/site'

type Post = {
  slug: string
  tag: string
  date: string
  title: string
  excerpt: string
  readTime: string
  color: string
  author: string
  authorRole: string
  featured?: boolean
  html: string
}

export const POSTS: Post[] = [...allBlogs]

export function BlogPage() {
  const [filter, setFilter] = useState('All')
  const [activePost, setActivePost] = useState<Post | null>(null)

  const tags = useMemo(() => ['All', ...new Set(POSTS.map((post) => post.tag))], [])
  const featured = POSTS.find((post) => post.featured) ?? POSTS[0]
  const rest = POSTS.filter((post) => !post.featured && (filter === 'All' || post.tag === filter))

  function openPost(post: Post) {
    startTransition(() => setActivePost(post))
  }

  function closePost(next?: Post) {
    startTransition(() => setActivePost(next ?? null))
  }

  return (
    <div className="min-h-screen">
      <SiteHeader current="blog" crumb={activePost ? `Blog / ${activePost.tag}` : 'Blog'} />
      <main className="flex-1">
        {activePost ? (
          <PostPage post={activePost} onOpen={openPost} onBack={closePost} />
        ) : (
          <div className="mx-auto max-w-[1480px] px-5 pb-24 pt-16 sm:px-8 lg:px-12 lg:pt-20">
            <div className="mb-14">
              <div className="font-mono text-[11px] font-bold uppercase tracking-[0.16em] text-brand-300">Gun Blog</div>
              <h1 className="mt-3 font-syne text-[clamp(2.4rem,5vw,3.6rem)] font-extrabold leading-[1] tracking-[-0.05em] text-white">
                What&apos;s new in Gun
              </h1>
              <p className="mt-3 max-w-xl text-[15px] leading-7 text-white/60">
                Release notes, deep dives, and stories from the compiler and runtime work that turns JavaScript into Go.
              </p>
            </div>

            <FeaturedCard post={featured} onClick={() => openPost(featured)} />

            <div className="mt-10 flex flex-wrap gap-2">
              {tags.map((tag) => (
                <button
                  key={tag}
                  type="button"
                  onClick={() => setFilter(tag)}
                  className={filter === tag ? 'rounded-full border border-brand-300/45 bg-brand-500/18 px-4 py-2 text-sm font-semibold text-white' : 'rounded-full border border-white/10 px-4 py-2 text-sm font-semibold text-white/60 transition hover:border-brand-300/35 hover:text-white'}
                >
                  {tag}
                </button>
              ))}
            </div>

            <div className="mt-8 grid gap-5 lg:grid-cols-3">
              {rest.map((post) => (
                <PostCard key={post.slug} post={post} onClick={() => openPost(post)} />
              ))}
            </div>

            <div className="mt-24 grid gap-8 rounded-[28px] border border-brand-300/35 bg-brand-500/10 px-6 py-10 lg:grid-cols-[1.2fr_1fr] lg:items-center lg:px-10">
              <div>
                <div className="inline-flex items-center gap-2 rounded-full border border-brand-300/25 bg-brand-500/10 px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-200">
                  Newsletter
                </div>
                <h2 className="mt-4 font-syne text-4xl font-extrabold tracking-[-0.05em] text-white">Stay in the loop.</h2>
                <p className="mt-3 max-w-lg text-[15px] leading-7 text-white/60">
                  New posts in your inbox, roughly weekly, with no marketing padding. Just release notes and engineering writeups.
                </p>
              </div>
              <form className="flex flex-col gap-3 sm:flex-row" onSubmit={(event) => event.preventDefault()}>
                <input
                  type="email"
                  placeholder="you@example.com"
                  className="min-w-0 flex-1 rounded-xl border border-brand-300/35 bg-black/40 px-4 py-3 text-sm text-white outline-none placeholder:text-white/35 focus:border-brand-200"
                />
                <button className="rounded-xl bg-brand-500 px-5 py-3 text-sm font-semibold text-white shadow-[0_0_24px_rgba(66,68,147,0.45)] transition hover:bg-brand-400">
                  Subscribe {'->'}
                </button>
              </form>
            </div>
          </div>
        )}
      </main>
      <SiteFooter />
    </div>
  )
}

function FeaturedCard({ post, onClick }: { post: Post; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="group relative block w-full overflow-hidden rounded-[28px] border border-brand-300/30 bg-brand-500/10 px-6 py-8 text-left transition hover:border-brand-200/45 hover:bg-brand-500/14 lg:px-10 lg:py-10"
    >
      <div className="absolute inset-x-0 top-0 h-px bg-[linear-gradient(90deg,transparent,#666bd7,transparent)] opacity-0 transition group-hover:opacity-100" />
      <div className="flex flex-wrap items-center gap-3 text-sm text-white/45">
        <Tag label={post.tag} color={post.color} />
        <span>{post.date}</span>
        <span>· {post.readTime} read</span>
      </div>
      <h2 className="mt-5 max-w-4xl font-syne text-[clamp(2rem,4vw,3rem)] font-extrabold leading-[1.05] tracking-[-0.05em] text-white">
        {post.title}
      </h2>
      <p className="mt-5 max-w-3xl text-[16px] leading-8 text-white/60">{post.excerpt}</p>
      <div className="mt-7 inline-flex items-center gap-2 text-sm font-semibold text-brand-100 transition group-hover:text-white">
        Read post
        <ArrowRight className="h-4 w-4" />
      </div>
    </button>
  )
}

function PostCard({ post, onClick }: { post: Post; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="group rounded-[24px] border border-white/10 bg-panel p-6 text-left transition hover:-translate-y-1 hover:border-brand-300/35 hover:bg-panel-hi"
    >
      <Tag label={post.tag} color={post.color} />
      <h3 className="mt-4 font-syne text-[1.35rem] font-bold leading-[1.15] tracking-[-0.04em] text-white">
        {post.title}
      </h3>
      <p className="mt-3 text-[14px] leading-7 text-white/60">{post.excerpt}</p>
      <div className="mt-5 flex items-center justify-between text-[12px] text-white/35">
        <span>{post.date}</span>
        <span>{post.readTime} read</span>
      </div>
    </button>
  )
}

function PostPage({ post, onOpen, onBack }: { post: Post; onOpen: (post: Post) => void; onBack: (post?: Post) => void }) {
  const currentIndex = POSTS.findIndex((entry) => entry.slug === post.slug)
  const newer = currentIndex > 0 ? POSTS[currentIndex - 1] : null
  const older = currentIndex < POSTS.length - 1 ? POSTS[currentIndex + 1] : null

  return (
    <div className="mx-auto max-w-[1120px] px-5 pb-24 pt-12 sm:px-8 lg:px-12 lg:pt-16">
      <button
        type="button"
        onClick={() => onBack()}
        className="inline-flex items-center gap-2 text-sm text-white/60 transition hover:text-white"
      >
        <ArrowLeft className="h-4 w-4" />
        All posts
      </button>

      <div className="mt-10 max-w-3xl">
        <div className="flex flex-wrap items-center gap-3 text-sm text-white/45">
          <Tag label={post.tag} color={post.color} />
          <span>{post.date}</span>
          <span>· {post.readTime} read</span>
        </div>
        <h1 className="mt-5 font-syne text-[clamp(2.4rem,5vw,3.9rem)] font-extrabold leading-[1.02] tracking-[-0.06em] text-white">
          {post.title}
        </h1>
        <p className="mt-5 text-lg leading-8 text-white/60">{post.excerpt}</p>
        <div className="mt-6 text-sm text-white/45">
          {post.author} / {post.authorRole}
        </div>
      </div>

      <div className="mt-10 h-px bg-[linear-gradient(90deg,rgba(102,107,215,0.7),transparent)]" />

      <article className="mt-10 max-w-3xl">
        <MarkdownContent html={post.html} />
      </article>

      <div className="mt-16 grid gap-4 border-t border-white/10 pt-8 md:grid-cols-2">
        {older ? <SiblingCard direction="older" post={older} onClick={() => onOpen(older)} /> : <div />}
        {newer ? <SiblingCard direction="newer" post={newer} onClick={() => onOpen(newer)} /> : <div />}
      </div>
    </div>
  )
}

function SiblingCard({ direction, post, onClick }: { direction: 'older' | 'newer'; post: Post; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={direction === 'newer' ? 'rounded-2xl border border-white/10 bg-brand-500/8 px-5 py-4 text-right transition hover:border-brand-300/35 hover:bg-brand-500/12' : 'rounded-2xl border border-white/10 bg-brand-500/8 px-5 py-4 text-left transition hover:border-brand-300/35 hover:bg-brand-500/12'}
    >
      <div className="font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-brand-300">
        {direction === 'older' ? '<- Previous' : 'Next ->'}
      </div>
      <div className="mt-1 text-sm font-semibold leading-6 text-white">{post.title}</div>
    </button>
  )
}
