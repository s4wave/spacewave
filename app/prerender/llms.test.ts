import { describe, expect, it } from 'vitest'

import type { DocPage } from '../docs/types.js'
import { buildLlmsFiles } from './llms.js'

function page(url: string, title: string, body: string): DocPage {
  const [, , site, section, slug] = url.split('/')
  return {
    slug,
    url,
    title,
    site,
    section,
    order: 1,
    summary: `${title} summary.`,
    body,
    filename: `01-${slug}.md`,
  }
}

describe('buildLlmsFiles', () => {
  const docs = [
    page('/docs/users/start/welcome', 'Welcome', 'See [basics](/docs/x).'),
    page('/docs/developers/cli/cli-reference', 'CLI Reference', 'Commands.'),
  ]
  const files = buildLlmsFiles(
    '# Spacewave\n\nGuide.\n',
    docs,
    'https://s.test',
  )

  it('appends a docs index with absolute links to the guide', () => {
    expect(files.llms.startsWith('# Spacewave\n\nGuide.\n\n## Docs')).toBe(true)
    expect(files.llms).toContain(
      '- [All docs in one file](https://s.test/llms-full.txt)',
    )
    expect(files.llms).toContain(
      '- [Welcome](https://s.test/docs/users/start/welcome): Welcome summary.',
    )
  })

  it('puts the CLI reference first in the full file', () => {
    expect(files.full.indexOf('# CLI Reference')).toBeLessThan(
      files.full.indexOf('# Welcome'),
    )
    expect(files.full).toContain('See [basics](https://s.test/docs/x).')
  })
})
