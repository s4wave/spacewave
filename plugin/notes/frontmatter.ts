import { parse as parseYaml } from 'yaml'

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
  const normalized = content.replace(/\r\n/g, '\n')
  if (!normalized.startsWith('---\n')) {
    return {
      frontmatter: {},
      rawFrontmatter: '',
      body: content,
    }
  }

  const closeIdx = normalized.indexOf('\n---', 4)
  if (closeIdx === -1) {
    return {
      frontmatter: {},
      rawFrontmatter: '',
      body: content,
    }
  }

  const raw = normalized.slice(4, closeIdx)
  const after = normalized.slice(closeIdx + 4)
  const body = after.startsWith('\n') ? after.slice(1) : after
  const parsed: unknown = parseYaml(raw)
  const frontmatter =
    parsed && typeof parsed === 'object' ? (parsed as Frontmatter) : {}

  return {
    frontmatter: normalizeFrontmatter(frontmatter),
    rawFrontmatter: '---\n' + raw + '\n---\n',
    body,
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

function normalizeFrontmatter(frontmatter: Frontmatter): Frontmatter {
  return {
    ...frontmatter,
    tags: normalizeStringArray(frontmatter.tags),
    categories: normalizeStringArray(frontmatter.categories),
    aliases: normalizeStringArray(frontmatter.aliases),
    author: normalizeStringArray(frontmatter.author),
    topics: normalizeStringArray(frontmatter.topics),
  }
}

function normalizeStringArray(value: unknown): string[] | undefined {
  if (value === undefined || value === null) return undefined
  if (Array.isArray(value)) {
    const values = value
      .map(normalizeStringValue)
      .filter((item): item is string => !!item)
    return values.length > 0 ? values : undefined
  }
  const text = normalizeStringValue(value)
  if (!text) return undefined
  return [text]
}

function normalizeStringValue(value: unknown): string | undefined {
  if (typeof value === 'string') return value.trim()
  if (
    typeof value === 'number' ||
    typeof value === 'boolean' ||
    typeof value === 'bigint'
  ) {
    return String(value).trim()
  }
  return undefined
}
