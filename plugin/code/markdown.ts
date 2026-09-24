import type { MarkdownToJSX } from 'markdown-to-jsx'

import { CodePreBlock } from './CodePreBlock.js'

// markdownCodeOptions are markdown-to-jsx options that highlight fenced code.
export const markdownCodeOptions: MarkdownToJSX.Options = {
  overrides: {
    pre: { component: CodePreBlock },
  },
}
