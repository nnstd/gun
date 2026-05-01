import { createFileRoute, redirect } from '@tanstack/react-router'
import { DocsPage, SLUG_TO_TITLE } from '../../pages/docs-page'

export const Route = createFileRoute('/docs/$slug')({
  beforeLoad: ({ params }) => {
    if (!SLUG_TO_TITLE[params.slug]) {
      throw redirect({ to: '/docs/introduction' })
    }
  },
  component: DocsSlugPage,
})

function DocsSlugPage() {
  const { slug } = Route.useParams()
  const title = SLUG_TO_TITLE[slug] ?? 'Introduction'
  return <DocsPage active={title} />
}
