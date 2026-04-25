import { codeToHtml } from 'shiki'
import { GUN_SHIKI_THEME, SHIKI_LANGUAGE_MAP } from './shiki-theme'

type DemoLanguage = 'js' | 'go'

const CACHE = new Map<string, string>()

function normalizePre(html: string) {
  return html
    .replace(/<pre([^>]*)class="([^"]*\bshiki\b[^"]*)"([^>]*)style="[^"]*"([^>]*)>/, '<pre$1class="$2"$3$4>')
    .replace(/<pre class="([^"]*\bshiki\b[^"]*)">/, '<pre class="$1 gun-landing-demo">')
    .replace(/<code>/, '<code class="shiki-demo__code">')
}

export async function highlightDemoCode(code: string, lang: DemoLanguage) {
  const key = `${lang}:${code}`
  const cached = CACHE.get(key)
  if (cached) return cached

  const html = await codeToHtml(code, {
    lang: SHIKI_LANGUAGE_MAP[lang],
    theme: GUN_SHIKI_THEME,
  })

  const normalized = normalizePre(html)
  CACHE.set(key, normalized)
  return normalized
}
