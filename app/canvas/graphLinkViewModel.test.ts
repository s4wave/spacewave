import { describe, expect, it } from 'vitest'

import {
  buildGraphLinkViewModel,
  getSelectedGraphNodes,
  type GraphLookupResult,
} from './graphLinkViewModel.js'
import type { CanvasNodeData } from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'n1',
    x: 10,
    y: 20,
    width: 100,
    height: 80,
    zIndex: 0,
    type: 'world_object',
    objectKey: 'objects/a',
    ...overrides,
  }
}

describe('graphLinkViewModel', () => {
  it('selects world object nodes and derives graph IRIs', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', objectKey: 'objects/a' })],
      ['b', makeNode({ id: 'b', type: 'text', objectKey: undefined })],
    ])

    expect(getSelectedGraphNodes(new Set(['a', 'b']), nodes)).toEqual([
      {
        nodeId: 'a',
        node: nodes.get('a'),
        iri: '<objects/a>',
      },
    ])
  })

  it('builds structured outgoing graph links with stable render keys', () => {
    const selected = {
      nodeId: 'a',
      node: makeNode({ id: 'a', objectKey: 'objects/a' }),
      iri: '<objects/a>',
    }
    const results: GraphLookupResult[] = [
      {
        selected,
        outgoing: [
          {
            subject: '<objects/a>',
            predicate: '<relatedTo>',
            obj: '<objects/b>',
            label: 'main',
          },
        ],
        incoming: [],
      },
    ]

    const links = buildGraphLinkViewModel(results, new Map())

    expect(links).toHaveLength(1)
    expect(links[0]).toMatchObject({
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
      label: 'main',
      sourceNodeId: 'a',
      sourceObjectKey: 'objects/a',
      sourceGroupKey: 'a',
      sourceGroupIndex: 0,
      sourceGroupOffset: 0,
      outgoingTruncated: false,
      incomingTruncated: false,
      hiddenCount: 0,
      direction: 'out',
      linkedObjectKey: 'objects/b',
      linkedObjectLabel: 'objects/b',
      hideable: true,
      userRemovable: true,
      protected: false,
      ownerManaged: false,
      stubX: 260,
      stubY: 60,
    })
    expect(links[0].renderKey).toBe(
      JSON.stringify([
        'a',
        'out',
        '<objects/a>',
        '<relatedTo>',
        '<objects/b>',
        'main',
      ]),
    )
  })

  it('builds incoming links to existing canvas nodes', () => {
    const selected = {
      nodeId: 'b',
      node: makeNode({ id: 'b', objectKey: 'objects/b' }),
      iri: '<objects/b>',
    }
    const results: GraphLookupResult[] = [
      {
        selected,
        outgoing: [],
        incoming: [
          {
            subject: '<objects/a>',
            predicate: '<relatedTo>',
            obj: '<objects/b>',
          },
        ],
      },
    ]

    const links = buildGraphLinkViewModel(
      results,
      new Map([['objects/a', 'node-a']]),
      {
        objectMetadata: new Map([
          [
            'objects/a',
            {
              label: 'Linked A',
              type: 'git/repo',
              typeLabel: 'Git Repository',
            },
          ],
        ]),
      },
    )

    expect(links).toEqual([
      {
        renderKey: JSON.stringify([
          'b',
          'in',
          '<objects/a>',
          '<relatedTo>',
          '<objects/b>',
          '',
        ]),
        subject: '<objects/a>',
        predicate: '<relatedTo>',
        object: '<objects/b>',
        label: undefined,
        sourceNodeId: 'b',
        sourceObjectKey: 'objects/b',
        sourceGroupKey: 'b',
        sourceGroupIndex: 0,
        sourceGroupOffset: 0,
        outgoingTruncated: false,
        incomingTruncated: false,
        hiddenCount: 0,
        direction: 'in',
        linkedObjectKey: 'objects/a',
        linkedObjectLabel: 'Linked A',
        linkedObjectType: 'git/repo',
        linkedObjectTypeLabel: 'Git Repository',
        hideable: true,
        userRemovable: true,
        protected: false,
        ownerManaged: false,
        targetNodeId: 'node-a',
        stubX: undefined,
        stubY: undefined,
      },
    ])
  })

  it('filters hidden and protected graph links before rendering', () => {
    const selected = {
      nodeId: 'a',
      node: makeNode({ id: 'a', objectKey: 'objects/a' }),
      iri: '<objects/a>',
    }
    const results: GraphLookupResult[] = [
      {
        selected,
        outgoing: [
          {
            subject: '<objects/a>',
            predicate: '<hidden>',
            obj: '<objects/b>',
          },
          {
            subject: '<objects/a>',
            predicate: '<type>',
            obj: '<types/git/repo>',
          },
          {
            subject: '<objects/a>',
            predicate: '<parent>',
            obj: '<objects/c>',
          },
        ],
        incoming: [],
      },
    ]

    const links = buildGraphLinkViewModel(results, new Map(), {
      hiddenGraphLinks: [
        {
          subject: '<objects/a>',
          predicate: '<hidden>',
          object: '<objects/b>',
        },
      ],
    })

    expect(links).toHaveLength(1)
    expect(links[0]).toMatchObject({
      predicate: '<parent>',
      sourceGroupKey: 'a',
      sourceGroupIndex: 0,
      sourceGroupOffset: 0,
      outgoingTruncated: false,
      incomingTruncated: false,
      hiddenCount: 1,
      linkedObjectLabel: 'objects/c',
      hideable: true,
      userRemovable: false,
      protected: false,
      ownerManaged: true,
    })
  })

  it('uses per-source deterministic placement slots', () => {
    const sourceA = {
      nodeId: 'a',
      node: makeNode({ id: 'a', objectKey: 'objects/a', x: 0, y: 0 }),
      iri: '<objects/a>',
    }
    const sourceC = {
      nodeId: 'c',
      node: makeNode({ id: 'c', objectKey: 'objects/c', x: 500, y: 100 }),
      iri: '<objects/c>',
    }

    const links = buildGraphLinkViewModel(
      [
        {
          selected: sourceA,
          outgoing: [
            {
              subject: '<objects/a>',
              predicate: '<relatedTo>',
              obj: '<objects/b>',
            },
          ],
          incoming: [],
        },
        {
          selected: sourceC,
          outgoing: [
            {
              subject: '<objects/c>',
              predicate: '<relatedTo>',
              obj: '<objects/d>',
            },
            {
              subject: '<objects/c>',
              predicate: '<relatedTo>',
              obj: '<objects/e>',
            },
          ],
          incoming: [],
        },
      ],
      new Map(),
    )

    expect(links.map((link) => link.sourceGroupIndex)).toEqual([0, 1, 1])
    expect(links.map((link) => link.sourceGroupOffset)).toEqual([0, 0, 1])
    expect(links.map((link) => link.stubX)).toEqual([250, 750, 810])
  })

  it('deduplicates the same graph quad across selected sources', () => {
    const sourceA = {
      nodeId: 'a',
      node: makeNode({ id: 'a', objectKey: 'objects/a' }),
      iri: '<objects/a>',
    }
    const sourceB = {
      nodeId: 'b',
      node: makeNode({ id: 'b', objectKey: 'objects/b' }),
      iri: '<objects/b>',
    }

    const links = buildGraphLinkViewModel(
      [
        {
          selected: sourceA,
          outgoing: [
            {
              subject: '<objects/a>',
              predicate: '<relatedTo>',
              obj: '<objects/b>',
            },
          ],
          incoming: [],
        },
        {
          selected: sourceB,
          outgoing: [],
          incoming: [
            {
              subject: '<objects/a>',
              predicate: '<relatedTo>',
              obj: '<objects/b>',
            },
          ],
        },
      ],
      new Map(),
    )

    expect(links).toHaveLength(1)
    expect(links[0].sourceNodeId).toBe('a')
  })

  it('carries truncation state from lookup results', () => {
    const selected = {
      nodeId: 'a',
      node: makeNode({ id: 'a', objectKey: 'objects/a' }),
      iri: '<objects/a>',
    }

    const links = buildGraphLinkViewModel(
      [
        {
          selected,
          outgoing: [
            {
              subject: '<objects/a>',
              predicate: '<relatedTo>',
              obj: '<objects/b>',
            },
          ],
          incoming: [],
          outgoingTruncated: true,
        },
      ],
      new Map(),
    )

    expect(links[0]).toMatchObject({
      outgoingTruncated: true,
      incomingTruncated: false,
      hiddenCount: 0,
    })
  })
})
