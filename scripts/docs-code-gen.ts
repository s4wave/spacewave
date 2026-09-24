// Highlights every docs code fence at build time so the app bundle carries
// the rendered HTML instead of the Shiki runtime and its grammars.

import { readFile, writeFile } from 'node:fs/promises'

import { Glob } from 'bun'
import Markdown from 'markdown-to-jsx'
import { createElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { createHighlighter } from 'shiki'

import {
  codeFenceKey,
  readCodeFence,
  type CodeFence,
} from '../app/docs/code-fence.js'

const contentDir = new URL('../app/docs/content/', import.meta.url).pathname

// Collect fences with the same markdown parser and reader the docs render with.
const fences = new Map<string, CodeFence>()
const collect = ({ children }: { children?: ReactNode }) => {
  const fence = readCodeFence(children)
  if (fence) fences.set(codeFenceKey(fence), fence)
  return null
}
const paths = Array.from(new Glob('**/*.md').scanSync(contentDir)).sort()
for (const path of paths) {
  const raw = await readFile(contentDir + path, 'utf-8')
  renderToStaticMarkup(
    createElement(Markdown, { options: { overrides: { pre: collect } } }, raw),
  )
}

const highlighter = await createHighlighter({
  themes: ['vesper'],
  langs: Array.from(new Set(Array.from(fences.values(), (f) => f.lang))).filter(
    (lang) => lang !== 'text',
  ),
})
try {
  const highlights = Array.from(fences.values(), (fence) => ({
    ...fence,
    html: highlighter.codeToHtml(fence.code, {
      lang: fence.lang,
      theme: 'vesper',
    }),
  }))
  await writeFile(
    new URL('../app/docs/code-highlight.generated.json', import.meta.url),
    JSON.stringify(highlights, null, 2) + '\n',
  )
} finally {
  highlighter.dispose()
}
