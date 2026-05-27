import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import type { DocPage } from './types.js'

// getRawMarkdownUrl returns the canonical raw source URL for a docs page.
export function getRawMarkdownUrl(doc: DocPage): string {
  const rawBase = GITHUB_REPO_URL.replace(
    'https://github.com/',
    'https://raw.githubusercontent.com/',
  )
  return `${rawBase}/master/app/docs/content/${doc.site}/${doc.section}/${doc.filename}`
}
