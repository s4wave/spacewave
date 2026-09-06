import { writeFile } from 'node:fs/promises'

import { createHighlighter } from 'shiki'

import { demoCode } from '../app/landing/demo-code.js'

const highlighter = await createHighlighter({
  themes: ['vesper'],
  langs: ['typescript', 'shellscript'],
})

try {
  const highlights = Object.values(demoCode).map(({ code, lang }) => ({
    code,
    lang,
    html: highlighter.codeToHtml(code, { lang, theme: 'vesper' }),
  }))
  await writeFile(
    new URL('../app/landing/demo-code.generated.json', import.meta.url),
    JSON.stringify(highlights, null, 2) + '\n',
  )
} finally {
  highlighter.dispose()
}
