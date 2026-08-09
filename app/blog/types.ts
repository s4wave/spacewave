import type { Author } from './authors.js'

export interface BlogPost {
  slug: string
  url: string
  title: string
  date: string
  author: Author
  authorSlug: string
  summary: string
  tags: string[]
  draft: boolean
  ogImage?: string
  body: string
}
