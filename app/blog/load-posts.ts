import { authors } from './authors.js'
import type { BlogPost, BlogPostFrontmatter } from './types.js'

// postModules imports all .md files from the posts directory.
// Vite's import.meta.glob handles this at build time.
const postModules = import.meta.glob('./posts/*.md', {
  query: '?raw',
  eager: true,
  import: 'default',
})

// parseFrontmatter extracts YAML frontmatter from a markdown string.
function parseFrontmatter(raw: string): {
  data: BlogPostFrontmatter
  content: string
} {
  const match = raw.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)
  if (!match) return { data: {} as BlogPostFrontmatter, content: raw }

  const yamlBlock = match[1]
  const content = match[2]

  // Simple YAML parser for flat frontmatter.
  const data: Record<string, unknown> = {}
  for (const line of yamlBlock.split('\n')) {
    const colonIdx = line.indexOf(':')
    if (colonIdx === -1) continue
    const key = line.slice(0, colonIdx).trim()
    let value: unknown = line.slice(colonIdx + 1).trim()

    // Parse arrays: [tag1, tag2]
    if (
      typeof value === 'string' &&
      value.startsWith('[') &&
      value.endsWith(']')
    ) {
      value = value
        .slice(1, -1)
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
    }
    // Parse booleans.
    if (value === 'true') value = true
    if (value === 'false') value = false

    data[key] = value
  }

  return { data: data as unknown as BlogPostFrontmatter, content }
}

// cachedPosts holds the parsed posts after first load.
let cachedPosts: BlogPost[] | null = null

// loadPosts parses all blog posts from the imported modules.
export function loadPosts(): BlogPost[] {
  if (cachedPosts) return cachedPosts

  const posts: BlogPost[] = []

  for (const [path, raw] of Object.entries(postModules)) {
    const { data: fm, content } = parseFrontmatter(raw as string)

    if (!fm.title || !fm.date || !fm.author || !fm.summary) continue
    if (fm.draft === true) continue

    // Derive slug from filename.
    const filename = path.split('/').pop()?.replace(/\.md$/, '') ?? ''
    const slug = filename.replace(/^\d{4}-\d{2}-\d{2}-/, '')

    // Derive URL from date.
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
      tags: Array.isArray(fm.tags) ? fm.tags : [],
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
