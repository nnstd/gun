import { defineCollection, defineConfig } from '@content-collections/core'
import { compileMarkdown } from '@content-collections/markdown'
import { codeToHtml } from 'shiki'
import { z } from 'zod'
import { GUN_SHIKI_THEME, SHIKI_LANGUAGE_MAP } from './src/lib/shiki-theme'

async function highlightMarkdownCode(html: string) {
  const matches = Array.from(html.matchAll(/<pre><code class="language-([^"]+)">([\s\S]*?)<\/code><\/pre>/g))
  if (matches.length === 0) return html

  let result = html

  for (const match of matches) {
    const [fullMatch, language, encodedCode] = match
    const normalizedLanguage = SHIKI_LANGUAGE_MAP[language] ?? language
    const decodedCode = encodedCode
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&amp;/g, '&')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")

    const highlighted = await codeToHtml(decodedCode.replace(/\n$/, ''), {
      lang: normalizedLanguage,
      theme: GUN_SHIKI_THEME,
    })

    const normalized = highlighted
      .replace(/<pre([^>]*)class="([^"]*\bshiki\b[^"]*)"([^>]*)style="[^"]*"([^>]*)>/, '<pre$1class="$2"$3$4>')
      .replace(/<pre class="([^"]*\bshiki\b[^"]*)">/, '<pre class="$1 markdown-shiki">')
      .replace(/<code>/, `<code class="markdown-shiki__code language-${language}">`)

    result = result.replace(fullMatch, normalized)
  }

  return result
}

const blog = defineCollection({
  name: 'blog',
  directory: 'src/content/blog',
  include: '**/*.md',
  schema: z.object({
    title: z.string(),
    slug: z.string(),
    tag: z.string(),
    date: z.string(),
    excerpt: z.string(),
    readTime: z.string(),
    color: z.string(),
    author: z.string(),
    authorRole: z.string(),
    featured: z.boolean().optional().default(false),
    content: z.string(),
  }),
  transform: async (document, context) => ({
    ...document,
    html: await highlightMarkdownCode(await compileMarkdown(context, document)),
  }),
})

const docs = defineCollection({
  name: 'doc',
  directory: 'src/content/docs',
  include: '**/*.md',
  schema: z.object({
    title: z.string(),
    lead: z.string().optional(),
    sections: z.array(z.string()).default([]),
    content: z.string(),
  }),
  transform: async (document, context) => ({
    ...document,
    html: await highlightMarkdownCode(await compileMarkdown(context, document)),
  }),
})

export default defineConfig({
  content: [blog, docs],
})
