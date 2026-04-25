import type { Author } from './authors.js'

// BlogPost represents a parsed and compiled blog post.
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

// BlogPostFrontmatter represents the YAML frontmatter of a blog post.
export interface BlogPostFrontmatter {
  title: string
  date: string
  author: string
  summary: string
  tags?: string[]
  ogImage?: string
  draft?: boolean
}
