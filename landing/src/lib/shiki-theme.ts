export const GUN_SHIKI_THEME = {
  name: 'gun-landing-demo',
  type: 'dark' as const,
  colors: {
    'editor.background': '#0d0d1a',
    'editor.foreground': '#e8e8f4',
  },
  tokenColors: [
    { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: '#44445a', fontStyle: 'italic' } },
    { scope: ['keyword', 'storage', 'keyword.control', 'keyword.operator.word'], settings: { foreground: '#9d9dff' } },
    { scope: ['entity.name.function', 'support.function', 'meta.function-call', 'variable.function'], settings: { foreground: '#7fd4f8' } },
    { scope: ['string', 'string.quoted', 'string.template', 'markup.inline.raw.string.markdown'], settings: { foreground: '#f5c542' } },
    { scope: ['constant.numeric', 'number'], settings: { foreground: '#f0a060' } },
    { scope: ['storage.type', 'entity.name.type', 'support.type'], settings: { foreground: '#4eca8a' } },
    { scope: ['punctuation', 'meta.brace', 'delimiter', 'keyword.operator', 'punctuation.separator'], settings: { foreground: '#6666aa' } },
    { scope: ['variable', 'meta.definition.variable'], settings: { foreground: '#e8e8f4' } },
  ],
}

export const SHIKI_LANGUAGE_MAP: Record<string, string> = {
  js: 'javascript',
  javascript: 'javascript',
  ts: 'typescript',
  typescript: 'typescript',
  go: 'go',
  bash: 'bash',
  sh: 'bash',
  shell: 'bash',
}
