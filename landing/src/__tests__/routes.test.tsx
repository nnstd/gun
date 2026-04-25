import { describe, expect, it } from 'vitest'
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
    expect(ALL_PAGES[0]).toBe('Introduction')
    expect(ALL_PAGES).toContain('CLI Reference')
    expect(DOC_PAGES.Installation?.lead).toMatch(/npm package/i)
    expect(DOC_PAGES['Quick Start']?.sections).toContain('Build a binary')
  })

  it('keeps blog metadata and featured post selection intact', () => {
    expect(POSTS.some((post) => post.featured && post.slug === 'introducing-gun')).toBe(true)
    expect(POSTS).toHaveLength(6)
    expect(POSTS.map((post) => post.tag)).toContain('Deep Dive')
  })
})
