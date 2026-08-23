import { getSectionKey, sectionDefs, siteDefs } from './sections.js'
import type { DocPage, DocFrontmatter } from './types.js'

// docModules imports all .md files from the content directory.
// Vite's import.meta.glob handles this at build time.
const docModules = import.meta.glob('./content/**/*.md', {
  query: '?raw',
  eager: true,
  import: 'default',
})

// parseFrontmatter extracts YAML frontmatter from a markdown string.
function parseFrontmatter(raw: string): {
  data: DocFrontmatter
  content: string
} {
  const match = raw.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)
  if (!match) return { data: {} as DocFrontmatter, content: raw }

  const yamlBlock = match[1]
  const content = match[2]

  // Simple YAML parser for flat frontmatter.
  const data: Record<string, unknown> = {}
  for (const line of yamlBlock.split('\n')) {
    const colonIdx = line.search(':')
    if (colonIdx === -1) continue
    const key = line.slice(0, colonIdx).trim()
    let value: unknown = line.slice(colonIdx + 1).trim()

    // Parse numbers.
    if (typeof value === 'string' && /^\d+$/.test(value)) {
      value = parseInt(value, 10)
    }
    // Parse booleans.
    if (value === 'true') value = true
    if (value === 'false') value = false

    data[key] = value
  }

  return { data: data as unknown as DocFrontmatter, content }
}

// cachedDocs holds the parsed docs after first load.
let cachedDocs: DocPage[] | null = null

// loadDocs parses all documentation pages from the imported modules.
export function loadDocs(): DocPage[] {
  if (cachedDocs) return cachedDocs

  const docs: DocPage[] = []

  for (const [path, raw] of Object.entries(docModules)) {
    const { data: fm, content } = parseFrontmatter(raw as string)

    if (!fm.title || !fm.section || fm.order == null || !fm.summary) continue
    if (fm.draft === true) continue

    // Derive site and section from the content/{site}/{section}/{page}.md
    // directory contract.
    const parts = path.split('/')
    let contentIdx = -1
    for (const [idx, part] of parts.entries()) {
      if (part === 'content') {
        contentIdx = idx
        break
      }
    }
    if (parts.length - contentIdx - 1 < 3) continue
    const site = parts[contentIdx + 1]
    const section = parts[contentIdx + 2]

    // Derive slug from filename, stripping numeric prefix.
    const rawFilename = parts[parts.length - 1]
    const filename = rawFilename.replace(/\.md$/, '')
    const slug = filename.replace(/^\d+-/, '')

    const url = `/docs/${site}/${section}/${slug}`

    docs.push({
      slug,
      url,
      title: fm.title,
      site,
      section,
      order: fm.order,
      summary: fm.summary,
      body: content,
      filename: rawFilename,
    })
  }

  // Sort by site order, then section order, then page order.
  const siteOrder = new Map(siteDefs.map((s) => [s.id, s.order]))
  const sectionOrder = new Map(
    sectionDefs.map((s) => [getSectionKey(s.site, s.id), s.order]),
  )
  docs.sort((a, b) => {
    const siteA = siteOrder.get(a.site) ?? 99
    const siteB = siteOrder.get(b.site) ?? 99
    if (siteA !== siteB) return siteA - siteB
    const sa = sectionOrder.get(getSectionKey(a.site, a.section)) ?? 99
    const sb = sectionOrder.get(getSectionKey(b.site, b.section)) ?? 99
    if (sa !== sb) return sa - sb
    return a.order - b.order
  })

  cachedDocs = docs
  return docs
}
