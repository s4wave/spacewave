import { siteDefs } from '../docs/sections.js'
import type { DocPage } from '../docs/types.js'

// LLMS_FULL_LEAD lists the pages llms-full.txt puts first, in order, because
// an agent connecting to Spacewave needs them before the rest.
const LLMS_FULL_LEAD = [
  '/docs/developers/cli/cli-reference',
  '/docs/users/cli/connect-an-agent',
  '/docs/users/cli/command-line-basics',
]

// LlmsFiles holds the generated agent guide files.
export interface LlmsFiles {
  // llms is /llms.txt: the operating guide followed by a docs index.
  llms: string
  // full is /llms-full.txt: every public docs page as one Markdown file.
  full: string
}

// buildLlmsFiles renders /llms.txt from the hand-written guide and an index
// of the docs, and /llms-full.txt from the docs bodies. Site-relative links
// become absolute so the files read correctly outside the site.
export function buildLlmsFiles(
  guide: string,
  docs: DocPage[],
  siteOrigin: string,
): LlmsFiles {
  const index = [`## Docs`, '']
  index.push(
    `- [All docs in one file](${siteOrigin}/llms-full.txt): every page below, CLI reference first.`,
  )
  for (const site of siteDefs) {
    const pages = docs.filter((doc) => doc.site === site.id)
    if (pages.length === 0) continue
    index.push('', `### ${site.label}`, '')
    for (const doc of pages) {
      index.push(`- [${doc.title}](${siteOrigin}${doc.url}): ${doc.summary}`)
    }
  }
  const llms = `${guide.trimEnd()}\n\n${index.join('\n')}\n`

  const lead = LLMS_FULL_LEAD.flatMap((url) =>
    docs.filter((doc) => doc.url === url),
  )
  const leadSet = new Set(lead)
  const ordered = [...lead, ...docs.filter((doc) => !leadSet.has(doc))]
  const pages = ordered.map(
    (doc) =>
      `# ${doc.title}\n\nSource: ${siteOrigin}${doc.url}\n\n${absoluteLinks(doc.body.trim(), siteOrigin)}\n`,
  )
  const full = `# Spacewave documentation\n\n> Every public Spacewave docs page. Start with ${siteOrigin}/llms.txt to connect an agent.\n\n${pages.join('\n')}`

  return { llms: absoluteLinks(llms, siteOrigin), full }
}

// absoluteLinks rewrites Markdown links to site paths as absolute URLs.
function absoluteLinks(markdown: string, siteOrigin: string): string {
  return markdown.replaceAll('](/', `](${siteOrigin}/`)
}
