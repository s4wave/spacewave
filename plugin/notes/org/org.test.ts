import { describe, expect, it } from 'vitest'

import {
  type OrgNode,
  findFirstOrgNode,
  parseOrg,
  serializeOrg,
  updateOrgBlock,
  updateOrgHeading,
} from './org.js'

const hardOrg = `#+TITLE: Corpus Floor
#+DATE: <2026-06-25 Thu>
#+SETUPFILE: ../../setup.org

* TODO Heading with tags :alpha:beta:
CLOSED: [2026-06-25 Thu 10:00]
:PROPERTIES:
:CUSTOM_ID: heading-id
:END:
:LOGBOOK:
CLOCK: [2026-06-25 Thu 10:00]--[2026-06-25 Thu 11:00] =>  1:00
:END:

| Name | Value |
|------+-------|
| [[file:other.org][Other]] | =code= |

#+begin_src ts
const value = 1
#+end_src

#+begin_example
literal *text*
#+end_example

#+begin_quote
quoted text
#+end_quote
`

const setupFiles = [
  '#+SETUPFILE: ../../setup.org\n* Depth\n',
  '#+SETUPFILE: ../../../setup.org\n* Depth\n',
  '#+SETUPFILE: ../setup.org\n* Depth\n',
  '#+SETUPFILE: setup.org\n* Depth\n',
]

describe('parseOrg and serializeOrg', () => {
  it('imports orga and round-trips the corpus floor without byte changes', () => {
    const document = parseOrg(hardOrg)

    expect(serializeOrg(document)).toBe(hardOrg)
  })

  it('preserves all setup-file depths byte-for-byte', () => {
    for (const source of setupFiles) {
      expect(serializeOrg(parseOrg(source))).toBe(source)
    }
  })

  it('models the corpus-floor org constructs with source spans', () => {
    const document = parseOrg(hardOrg)
    const nodes = collectOrgNodes(document.root)

    const headline = findFirstOrgNode(document, 'headline')
    expect(headline.keyword).toBe('TODO')
    expect(headline.tags).toEqual(['alpha', 'beta'])
    expect(headline.title).toBe('Heading with tags')

    expect(nodes.some((node) => node.type === 'planning')).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'drawer' &&
          node.source.includes(':CUSTOM_ID: heading-id'),
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'drawer' &&
          node.source.includes('CLOCK: [2026-06-25 Thu 10:00]'),
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'table' && node.source.includes('| Name | Value |'),
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'link' && node.source === '[[file:other.org][Other]]',
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'block' && node.source.startsWith('#+begin_src ts'),
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'block' && node.source.startsWith('#+begin_example'),
      ),
    ).toBe(true)
    expect(
      nodes.some(
        (node) =>
          node.type === 'block' && node.source.startsWith('#+begin_quote'),
      ),
    ).toBe(true)
  })

  it('emits changed headings through the explicit heading emitter only', () => {
    const document = parseOrg(hardOrg)
    const heading = findFirstOrgNode(document, 'headline')

    expect(heading).toBeTruthy()

    const updated = updateOrgHeading(document, heading.id, {
      keyword: 'DONE',
      title: 'Renamed heading',
      tags: ['alpha'],
    })

    expect(serializeOrg(updated)).toBe(
      hardOrg.replace(
        '* TODO Heading with tags :alpha:beta:',
        '* DONE Renamed heading :alpha:',
      ),
    )
  })

  it('emits changed source blocks through the explicit block emitter only', () => {
    const document = parseOrg(hardOrg)
    const block = findFirstOrgNode(document, 'block')

    const updated = updateOrgBlock(document, block.id, {
      name: 'src',
      params: ['ts'],
      value: 'const value = 2',
    })

    expect(serializeOrg(updated)).toBe(
      hardOrg.replace('const value = 1', 'const value = 2'),
    )
  })
})

function collectOrgNodes(node: OrgNode): OrgNode[] {
  const nodes = [node]
  for (const child of node.children) {
    nodes.push(...collectOrgNodes(child))
  }
  return nodes
}
