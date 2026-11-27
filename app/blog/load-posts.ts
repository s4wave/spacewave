import {
  frontmatterRecord,
  readFrontmatter,
} from '@s4wave/app/frontmatter/frontmatter.js'

import { authors } from './authors.js'
import type { BlogPost, BlogPostFrontmatter } from './types.js'

// postModules imports all .md files from the posts directory.
// Vite's import.meta.glob handles these imports at build time.
const postModules = import.meta.glob('./posts/*.md', {
  query: '?raw',
  eager: true,
  import: 'default',
})

// cachedPosts holds the parsed posts after the first load.
let cachedPosts: BlogPost[] | null = null

// loadPosts parses all blog posts from the imported modules.
export function loadPosts(): BlogPost[] {
  if (cachedPosts) return cachedPosts

  const posts: BlogPost[] = []

  for (const [path, raw] of Object.entries(postModules)) {
    // The frontmatter content mapper checks each post against
    // BlogPostFrontmatter during the type check, so the record has that shape.
    const parsed = readFrontmatter(raw as string)
    const fm = frontmatterRecord(parsed) as unknown as BlogPostFrontmatter
    const content = parsed.body
    if (fm.draft === true) continue

    // Derive the slug from the filename.
    const filename = path.split('/').pop()?.replace(/\.md$/, '') ?? ''
    const slug = filename.replace(/^\d{4}-\d{2}-\d{2}-/, '')

    // Derive the URL from the date.
    const dateParts = fm.date.split('-')
    const year = dateParts[0]
    const month = dateParts[1]
    const url = `/blog/${year}/${month}/${slug}`

    const author = authors[fm.author]

    posts.push({
      slug,
      url,
      title: fm.title,
      date: fm.date,
      author: author ?? { name: fm.author, avatar: '', url: '', bio: '' },
      authorSlug: fm.author,
      summary: fm.summary,
      tags: fm.tags ?? [],
      draft: false,
      ogImage: fm.ogImage,
      body: content,
    })
  }

  // Sort by date descending.
  posts.sort((a, b) => b.date.localeCompare(a.date))
  cachedPosts = posts
  return posts
}
