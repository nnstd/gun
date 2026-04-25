import { createFileRoute } from '@tanstack/react-router'
import { BlogPage } from '../pages/blog-page'

export const Route = createFileRoute('/blog')({ component: BlogPage })
