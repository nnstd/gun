import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { POSTS } from '../pages/blog-page'
import { ALL_PAGES, DOC_PAGES } from '../pages/docs-page'
import { BENCHMARK_ROWS, FAQS, PILLARS, TRUSTED_BY } from '../pages/landing-page'

describe('landing routes', () => {
  it('keeps the landing page content inventory intact', () => {
    expect(TRUSTED_BY).toContain('vercel')
    expect(BENCHMARK_ROWS).toHaveLength(4)
    expect(BENCHMARK_ROWS[0]?.label).toBe('HTTP req/sec')
    expect(PILLARS.some((pillar) => pillar.title === 'npm dependencies, transpiled in place')).toBe(true)
    expect(FAQS.some((item) => item.question === 'Will every npm package work?')).toBe(true)
  })

  it('keeps docs navigation and page content wired', () => {
    const docsDir = join(process.cwd(), 'src/content/docs')
    const files = readdirSync(docsDir).filter((file) => file.endsWith('.md'))

    expect(files).toHaveLength(20)
    expect(files).toContain('introduction.md')
    expect(ALL_PAGES[0]).toBe('Introduction')
    expect(ALL_PAGES).toContain('CLI Reference')
    expect(DOC_PAGES.Installation?.lead).toMatch(/npm package/i)
    expect(DOC_PAGES['Quick Start']?.sections).toContain('Build a binary')
  })

  it('keeps blog metadata and featured post selection intact', () => {
    const blogDir = join(process.cwd(), 'src/content/blog')
    const files = readdirSync(blogDir).filter((file) => file.endsWith('.md'))

    expect(files).toHaveLength(6)
    expect(files).toContain('introducing-gun.md')

    const introPost = readFileSync(join(blogDir, 'introducing-gun.md'), 'utf8')
    expect(introPost).toMatch(/slug: introducing-gun/)
    expect(introPost).toMatch(/## The problem/)

    expect(POSTS.some((post) => post.featured && post.slug === 'introducing-gun')).toBe(true)
  })
})
