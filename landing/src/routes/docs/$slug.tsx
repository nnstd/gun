import { createFileRoute, redirect } from '@tanstack/react-router'
import { DOC_PAGES, SLUG_TO_TITLE } from '../../pages/docs-page'
import { DocsPage } from '../../pages/docs-page'

export const Route = createFileRoute('/docs/$slug')({
  beforeLoad: ({ params }) => {
    if (!SLUG_TO_TITLE[params.slug]) {
      throw redirect({ to: '/docs/introduction' })
    }
  },
  loader: ({ params }) => {
    const title = SLUG_TO_TITLE[params.slug]
    return { title }
  },
  component: DocsSlugPage,
})

function DocsSlugPage() {
  const { slug } = Route.useParams()
  const { title } = Route.useLoaderData()
  return <DocsPage key={slug} active={title} />
}
