import { codeToHtml } from 'shiki'
import { GUN_SHIKI_THEME, SHIKI_LANGUAGE_MAP } from './shiki-theme'

const CACHE = new Map<string, string>()

function normalizeShikiBlock(html: string, language: string) {
  return html
    .replace(/<pre([^>]*)class="([^"]*\bshiki\b[^"]*)"([^>]*)style="[^"]*"([^>]*)>/, '<pre$1class="$2"$3$4>')
    .replace(/<pre class="([^"]*\bshiki\b[^"]*)">/, '<pre class="$1 code-shiki">')
    .replace(/<code>/, `<code class="code-shiki__code language-${language}">`)
}

export async function highlightCodeBlock(code: string, language: string) {
  const normalizedLanguage = SHIKI_LANGUAGE_MAP[language] ?? language
  const key = `${normalizedLanguage}:${code}`
  const cached = CACHE.get(key)
  if (cached) return cached

  const html = await codeToHtml(code, {
    lang: normalizedLanguage,
    theme: GUN_SHIKI_THEME,
  })

  const normalized = normalizeShikiBlock(html, language)
  CACHE.set(key, normalized)
  return normalized
}
