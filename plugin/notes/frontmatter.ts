import matter from 'gray-matter'

// Frontmatter contains parsed YAML frontmatter fields.
export interface Frontmatter {
  tags?: string[]
  categories?: string[]
  aliases?: string[]
  created?: string
  status?: string
  title?: string
  author?: string[]
  url?: string
  published?: string
  topics?: string[]
  [key: string]: unknown
}

// ParsedNote contains the separated frontmatter and body of a markdown note.
export interface ParsedNote {
  frontmatter: Frontmatter
  rawFrontmatter: string
  body: string
}

// parseNote separates YAML frontmatter from markdown body.
export function parseNote(content: string): ParsedNote {
  const parsed = matter(content)
  const data = parsed.data as Frontmatter

  // Reconstruct raw frontmatter string for round-trip.
  let rawFrontmatter = ''
  if (parsed.matter && parsed.matter.trim()) {
    rawFrontmatter = '---\n' + parsed.matter + '\n---\n'
  }

  return {
    frontmatter: data,
    rawFrontmatter,
    body: parsed.content,
  }
}

// reassembleNote prepends frontmatter back to the body.
export function reassembleNote(rawFrontmatter: string, body: string): string {
  if (!rawFrontmatter) return body
  // Ensure single newline between frontmatter and body.
  const trimmedBody = body.replace(/^\n+/, '')
  return rawFrontmatter + '\n' + trimmedBody
}

// stripWikiLinks removes [[...]] bracket syntax from a string (Obsidian convention).
export function stripWikiLinks(value: string): string {
  return value.replace(/\[\[([^\]]+)\]\]/g, '$1')
}

// getFrontmatterTags returns normalized tag/topic labels from frontmatter.
export function getFrontmatterTags(frontmatter: Frontmatter): string[] {
  const combined = [...(frontmatter.tags ?? []), ...(frontmatter.topics ?? [])]
  const seen = new Set<string>()
  const tags: string[] = []

  for (const item of combined) {
    const tag = stripWikiLinks(String(item)).trim()
    if (!tag) continue
    const key = tag.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    tags.push(tag)
  }

  return tags
}

// normalizeFrontmatterStatus returns a normalized status filter value.
export function normalizeFrontmatterStatus(
  status: string | undefined,
): string | undefined {
  const value = status?.trim().toLowerCase()
  return value || undefined
}
