import { CodeHighlightNode, CodeNode } from '@lexical/code'
import { LinkNode } from '@lexical/link'
import { ListItemNode, ListNode } from '@lexical/list'
import { HeadingNode, QuoteNode, $isHeadingNode } from '@lexical/rich-text'
import { TableCellNode, TableNode, TableRowNode } from '@lexical/table'
import { $createTextNode, $getRoot, createEditor } from 'lexical'
import { describe, expect, it } from 'vitest'

import { OrgPassthroughNode } from '../editor/OrgPassthroughNode.js'
import { $convertFromOrgString, $convertToOrgString } from './lexical.js'

const EDITOR_NODES = [
  HeadingNode,
  QuoteNode,
  CodeNode,
  CodeHighlightNode,
  LinkNode,
  ListNode,
  ListItemNode,
  OrgPassthroughNode,
  TableNode,
  TableCellNode,
  TableRowNode,
]

describe('Lexical Org bridge', () => {
  it('edits a modeled heading while preserving passthrough constructs byte-for-byte', () => {
    const source = `#+TITLE: Bridge Proof

* TODO Original :alpha:
CLOSED: [2026-06-25 Thu 10:00]
:PROPERTIES:
:CUSTOM_ID: heading-id
:END:
:LOGBOOK:
CLOCK: [2026-06-25 Thu 10:00]--[2026-06-25 Thu 11:00] =>  1:00
:END:

#+begin_unknown
keep *raw* [[link]]
#+end_unknown
`

    const output = convertOrg(source, () => {
      const heading = $getRoot().getChildren().find($isHeadingNode)
      expect(heading).toBeDefined()
      heading!.clear()
      heading!.append($createTextNode('DONE Edited :alpha:'))
    })

    expect(output).toContain('* DONE Edited :alpha:\n')
    expect(output).toContain(
      'CLOSED: [2026-06-25 Thu 10:00]\n:PROPERTIES:\n:CUSTOM_ID: heading-id\n:END:\n:LOGBOOK:\nCLOCK: [2026-06-25 Thu 10:00]--[2026-06-25 Thu 11:00] =>  1:00\n:END:',
    )
    expect(output).toContain(
      '#+begin_unknown\nkeep *raw* [[link]]\n#+end_unknown',
    )
    expect(output.startsWith('#+TITLE: Bridge Proof\n\n')).toBe(true)
  })

  it('maps Org paragraphs, links, source blocks, tables, and simple lists onto Lexical nodes', () => {
    const source = `Paragraph with [[file:other.org][Other]] and *raw* emphasis.

#+begin_src ts
const value = 1
#+end_src

| Name | Value |
| [[file:other.org][Other]] | =code= |

- first
- second
`

    const output = convertOrg(source)

    expect(output).toContain(
      'Paragraph with [[file:other.org][Other]] and *raw* emphasis.\n',
    )
    expect(output).toContain('#+begin_src ts\nconst value = 1\n#+end_src\n')
    expect(output).toContain('| Name | Value |\n')
    expect(output).toContain('- first\n- second\n')
  })
})

function convertOrg(source: string, edit?: () => void): string {
  const editor = createEditor({ nodes: EDITOR_NODES })
  let output = ''
  editor.update(
    () => {
      $convertFromOrgString(source)
      edit?.()
      output = $convertToOrgString()
    },
    { discrete: true },
  )
  return output
}
