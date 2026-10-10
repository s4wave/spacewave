import { describe, expect, it } from 'vitest'
import { parse } from 'orga'

import {
  type OrgNode,
  findFirstOrgNode,
  getOrgBridgeSegments,
  parseOrg,
  readOrgLink,
  reassembleOrgMetadata,
  serializeOrg,
  splitOrgMetadata,
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
  it.each([
    [
      'two links with a wrapped second description',
      '- References: [[file:first.org][First]] and [[file:second.org::*Section][Second\n  reference]].\n',
    ],
    [
      'wrapped description followed by an empty line',
      '- Reference: [[file:other.org::*Section][Other\n  section]].\n\n',
    ],
    [
      'ordered item with emphasis and two wrapped links',
      '8. *Reference.* See [[file:other.org::78][Other\n   section]] and [[https://example.org][More,\n   detail]].\n',
    ],
    [
      'nested checked item with a wrapped link',
      '- Parent\n  1. [X] See [[file:other.org::*Section][Other\n     section]] for details.\n',
    ],
    [
      'wrapped description between same-line links',
      '- See [[file:first.org][First]], [[file:second.org][Second\n  reference]], [[file:third.org][Third]].\n',
    ],
    [
      'multiple wrapped descriptions in one item',
      '- See [[file:first.org][First\n  reference]], [[file:second.org][Second\n  reference]], and [[file:third.org][Third\n  reference]].\n',
    ],
  ])('parses %s without changing source bytes', (_name, source) => {
    // Exercise the parser without the editor or its metadata adapter.
    expect(parse(source).type).toBe('document')

    // Keep complete link spans and source bytes through the Notes adapter.
    const document = parseOrg(source)
    expect(findFirstOrgNode(document, 'link').source).toContain('[[')
    expect(serializeOrg(document)).toBe(source)
    expect(
      getOrgBridgeSegments(document)
        .map((segment) => segment.source)
        .join(''),
    ).toBe(source)
  })

  it('retains every line of a wrapped list link description', () => {
    const source = '- [[file:other.org][First\n  second\n  third]]\n'
    const link = findFirstOrgNode(parseOrg(source), 'link')

    expect(readOrgLink(link)).toEqual({
      url: 'file:other.org',
      text: 'First\n  second\n  third',
    })
  })

  it('parses a large shallow document without using a stack frame per token', () => {
    // Repeated paragraphs exercise document length without deep nesting.
    const source =
      '#+TITLE: Large synthetic document.\n\n' +
      'A resident paragraph for complete-read checks.\n'.repeat(13_000) +
      'continuous-line-'.repeat(2_000) +
      '\nEND OF OVERSIZED FIXTURE\n'
    expect(source.length).toBe(643_062)

    // Both the parser and the Notes adapter must reach the final line.
    const document = parseOrg(source)
    expect(document.root.position.end.offset).toBe(source.length)
    expect(serializeOrg(document)).toBe(source)
  }, 110_000)

  it('imports orga and round-trips the corpus floor without byte changes', () => {
    const document = parseOrg(hardOrg)

    expect(serializeOrg(document)).toBe(hardOrg)
  })

  it('preserves all setup-file depths byte-for-byte', () => {
    for (const source of setupFiles) {
      expect(serializeOrg(parseOrg(source))).toBe(source)
    }
  })

  it('splits and reassembles leading keyword metadata byte-for-byte', () => {
    // Parse leading metadata separately from the document body.
    const source =
      '#+TITLE: Split Proof\n#+SETUPFILE: ../../setup.org\n\n* Body\n'
    const split = splitOrgMetadata(source)

    // Require both spans to reassemble into the original source.
    expect(split.metadata).toBe(
      '#+TITLE: Split Proof\n#+SETUPFILE: ../../setup.org\n\n',
    )
    expect(split.body).toBe('* Body\n')
    expect(reassembleOrgMetadata(split.metadata, split.body)).toBe(source)
  })

  it('classifies bridge segments without turning passthrough into editor grammar', () => {
    // Collect editable and passthrough spans from the complete document.
    const document = parseOrg(hardOrg)
    const segments = getOrgBridgeSegments(document)

    // Keep metadata and drawers outside the modeled heading spans.
    expect(
      segments.some(
        (segment) =>
          segment.kind === 'modeled' && segment.modeledKind === 'headline',
      ),
    ).toBe(true)
    expect(
      segments.some(
        (segment) =>
          segment.kind === 'passthrough' &&
          segment.source.includes(':PROPERTIES:'),
      ),
    ).toBe(true)
    expect(
      segments.some(
        (segment) =>
          segment.kind === 'passthrough' &&
          segment.source.includes('CLOCK: [2026-06-25 Thu 10:00]'),
      ),
    ).toBe(true)
  })

  it('models the corpus-floor org constructs with source spans', () => {
    // Parse the document and retain its complete tree.
    const document = parseOrg(hardOrg)
    const nodes = collectOrgNodes(document.root)

    // Require the heading to retain its keyword, tags and title.
    const headline = findFirstOrgNode(document, 'headline')
    expect(headline.keyword).toBe('TODO')
    expect(headline.tags).toEqual(['alpha', 'beta'])
    expect(headline.title).toBe('Heading with tags')

    // Require each supported construct to retain its original source span.
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
    // Select the original heading from the parsed document.
    const document = parseOrg(hardOrg)
    const heading = findFirstOrgNode(document, 'headline')
    expect(heading).toBeTruthy()

    // Change the heading fields without changing other source spans.
    const updated = updateOrgHeading(document, heading.id, {
      keyword: 'DONE',
      title: 'Renamed heading',
      tags: ['alpha'],
    })

    // Compare the export with the original source and one replacement.
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
