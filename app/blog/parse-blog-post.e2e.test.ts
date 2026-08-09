import { expect, test } from 'vitest'

import { parseBlogPost } from './parse-blog-post.js'

test('executes the shared YAML blog parser in the built browser module', () => {
  const post = parseBlogPost(
    `---
title: "Browser: YAML"
date: 2026-05-18
author: paralin
summary: >-
  Folded browser
  summary.
tags: ["quoted tag", browser]
---
Body before fence.

\`\`\`yaml
---
fence: body
---
\`\`\`
`,
    '2026-05-18-browser-yaml.md',
  )

  expect(post).toMatchObject({
    title: 'Browser: YAML',
    date: '2026-05-18',
    summary: 'Folded browser summary.',
    tags: ['quoted tag', 'browser'],
    body: 'Body before fence.\n\n```yaml\n---\nfence: body\n---\n```\n',
  })
  expect(typeof globalThis.Buffer).toBe('undefined')
})
