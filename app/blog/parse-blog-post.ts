import { parse } from 'yaml'

import { authors } from './authors.js'
import type { BlogPost } from './types.js'

type Warn = (message: string) => void

interface ParsedSource {
  frontmatter: Record<string, unknown>
  body: string
}

function parseSource(raw: string): ParsedSource {
  const match = raw.match(
    /^---[^\S\r\n]*\r?\n([\s\S]*?)\r?\n(?:---|\.\.\.)[^\S\r\n]*(?:\r?\n|$)/,
  )
  if (!match) return { frontmatter: {}, body: raw }

  const value: unknown = parse(match[1])
  const frontmatter =
    value !== null && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {}
  return { frontmatter, body: raw.slice(match[0].length) }
}

export function parseBlogPost(
  raw: string,
  filename: string,
  warn?: Warn,
): BlogPost | null {
  const { frontmatter, body } = parseSource(raw)
  const title = typeof frontmatter.title === 'string' ? frontmatter.title : ''
  const authorSlug =
    typeof frontmatter.author === 'string' ? frontmatter.author : ''
  const summary =
    typeof frontmatter.summary === 'string' ? frontmatter.summary : ''
  const rawDate = frontmatter.date

  if (!title || !rawDate || !authorSlug || !summary) {
    warn?.(`[blog] Skipping ${filename}: missing required frontmatter`)
    return null
  }

  const date =
    rawDate instanceof Date
      ? rawDate.toISOString().slice(0, 10)
      : String(rawDate)
  const basename = filename.split('/').pop()?.replace(/\.md$/, '') ?? ''
  const slug = basename.replace(/^\d{4}-\d{2}-\d{2}-/, '')
  const [year, month] = date.split('-')
  const author = authors[authorSlug]

  if (!author) {
    warn?.(
      `[blog] Unknown author "${authorSlug}" in ${filename}, using fallback`,
    )
  }

  return {
    slug,
    url: `/blog/${year}/${month}/${slug}`,
    title,
    date,
    author: author ?? { name: authorSlug, avatar: '', url: '', bio: '' },
    authorSlug,
    summary,
    tags: Array.isArray(frontmatter.tags)
      ? frontmatter.tags.filter((tag): tag is string => typeof tag === 'string')
      : [],
    draft: frontmatter.draft === true,
    ogImage:
      typeof frontmatter.ogImage === 'string' ? frontmatter.ogImage : undefined,
    body,
  }
}
