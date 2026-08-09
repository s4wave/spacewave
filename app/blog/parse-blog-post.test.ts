import { describe, expect, it } from 'vitest'

import { parseBlogPost } from './parse-blog-post.js'

const source = `---
title: "Quoted: title"
date: 2026-05-18
author: paralin
summary: >-
  A multiline summary
  with YAML folding.
tags:
  - "release notes"
  - yaml
draft: false
---
Post body

\`\`\`yaml
---
inside: fence
---
\`\`\`
`

describe('parseBlogPost', () => {
  it('normalizes quoted, date, array, and multiline YAML frontmatter', () => {
    expect(parseBlogPost(source, '2026-05-18-parser-parity.md')).toMatchObject({
      slug: 'parser-parity',
      url: '/blog/2026/05/parser-parity',
      title: 'Quoted: title',
      date: '2026-05-18',
      authorSlug: 'paralin',
      summary: 'A multiline summary with YAML folding.',
      tags: ['release notes', 'yaml'],
      draft: false,
      body: 'Post body\n\n```yaml\n---\ninside: fence\n---\n```\n',
    })
  })
})
