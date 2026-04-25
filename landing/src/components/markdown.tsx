type MarkdownContentProps = {
  html: string
}

export function MarkdownContent({ html }: MarkdownContentProps) {
  return (
    <div
      className="max-w-none [&_h2]:mt-12 [&_h2]:font-syne [&_h2]:text-[1.75rem] [&_h2]:font-extrabold [&_h2]:tracking-[-0.04em] [&_h2]:text-white [&_p]:text-[16px] [&_p]:leading-8 [&_p]:text-white/65 [&_strong]:text-white [&_a]:text-brand-100 [&_ul]:text-white/65 [&_ol]:text-white/65 [&_li]:my-1 [&_blockquote]:my-6 [&_blockquote]:rounded-2xl [&_blockquote]:border [&_blockquote]:border-brand-300/30 [&_blockquote]:bg-brand-500/10 [&_blockquote]:px-4 [&_blockquote]:py-3 [&_blockquote]:text-white/75 [&_blockquote_p]:text-white/75 [&_pre]:my-6 [&_pre]:overflow-x-auto [&_pre]:rounded-2xl [&_pre]:border [&_pre]:border-white/10 [&_pre]:bg-black/55 [&_pre]:px-4 [&_pre]:py-4 [&_pre]:shadow-[0_12px_42px_rgba(0,0,0,0.35)] [&_pre_code]:block [&_pre_code]:whitespace-pre [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_pre_code]:font-mono [&_pre_code]:text-[13px] [&_pre_code]:leading-6 [&_pre_code]:text-white/90 [&_.markdown-shiki]:m-0 [&_.markdown-shiki]:bg-transparent [&_.markdown-shiki]:p-0 [&_.markdown-shiki]:font-mono [&_.markdown-shiki]:text-white/90 [&_.markdown-shiki__code]:block [&_.markdown-shiki__code]:whitespace-pre [&_.markdown-shiki__code]:font-mono [&_.markdown-shiki__code]:text-[13px] [&_.markdown-shiki__code]:leading-6 [&_.markdown-shiki__code]:text-white/90 [&_.markdown-shiki_.line]:block [&_.markdown-shiki_.line]:leading-6 [&_p_code]:rounded-md [&_p_code]:border [&_p_code]:border-brand-400/25 [&_p_code]:bg-brand-500/10 [&_p_code]:px-1.5 [&_p_code]:py-1 [&_p_code]:font-mono [&_p_code]:text-[13px] [&_p_code]:text-brand-100 [&_li_code]:rounded-md [&_li_code]:border [&_li_code]:border-brand-400/25 [&_li_code]:bg-brand-500/10 [&_li_code]:px-1.5 [&_li_code]:py-1 [&_li_code]:font-mono [&_li_code]:text-[13px] [&_li_code]:text-brand-100 [&_blockquote_code]:rounded-md [&_blockquote_code]:border [&_blockquote_code]:border-brand-400/25 [&_blockquote_code]:bg-brand-500/10 [&_blockquote_code]:px-1.5 [&_blockquote_code]:py-1 [&_blockquote_code]:font-mono [&_blockquote_code]:text-[13px] [&_blockquote_code]:text-brand-100"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
