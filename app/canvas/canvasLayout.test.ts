import { describe, expect, it } from 'vitest'

import { organizeCanvasNodes } from './canvasLayout.js'
import type { CanvasLayoutMetadataData, CanvasNodeData } from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'node',
    x: 900,
    y: 900,
    width: 200,
    height: 100,
    zIndex: 0,
    type: 'text',
    ...overrides,
  }
}

describe('organizeCanvasNodes', () => {
  it('lays out metadata-backed nodes by lane, rank, group, and stable id', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['proof', makeNode({ id: 'proof' })],
      ['source-b', makeNode({ id: 'source-b' })],
      ['source-a', makeNode({ id: 'source-a' })],
    ])
    const metadata = new Map<string, CanvasLayoutMetadataData>([
      [
        'proof',
        {
          lane: 'proof',
          rank: 2,
          group: 'evidence',
          stableNodeId: 'spell-run:proof',
        },
      ],
      [
        'source-b',
        {
          lane: 'source',
          rank: 0,
          group: 'intent',
          stableNodeId: 'spell-run:source-b',
        },
      ],
      [
        'source-a',
        {
          lane: 'source',
          rank: 0,
          group: 'intent',
          stableNodeId: 'spell-run:source-a',
        },
      ],
    ])

    const changed = organizeCanvasNodes(nodes, metadata)

    expect(changed.get('source-a')?.x).toBe(80)
    expect(changed.get('source-a')?.y).toBe(80)
    expect(changed.get('source-b')?.x).toBe(80)
    expect(changed.get('source-b')?.y).toBe(204)
    expect(changed.get('proof')?.x).toBe(380)
    expect(changed.get('proof')?.y).toBe(484)
  })

  it('sizes rank columns from the widest node in each rank', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['wide', makeNode({ id: 'wide', width: 420 })],
      ['next', makeNode({ id: 'next', width: 160 })],
    ])
    const metadata = new Map<string, CanvasLayoutMetadataData>([
      ['wide', { lane: 'source', rank: 0, stableNodeId: 'wide' }],
      ['next', { lane: 'source', rank: 1, stableNodeId: 'next' }],
    ])

    const changed = organizeCanvasNodes(nodes, metadata)

    expect(changed.get('wide')?.x).toBe(80)
    expect(changed.get('next')?.x).toBe(600)
  })

  it('ignores visual edges and nodes without layout metadata', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a' })],
      ['b', makeNode({ id: 'b', x: 42, y: 43 })],
    ])
    const metadata = new Map<string, CanvasLayoutMetadataData>([
      ['a', { lane: 'source', rank: 0, stableNodeId: 'a' }],
    ])
    const changed = organizeCanvasNodes(nodes, metadata)

    expect(changed.has('a')).toBe(true)
    expect(changed.has('b')).toBe(false)
  })

  it('returns no changed nodes when organized positions already match', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', x: 80, y: 80 })],
    ])
    const metadata = new Map<string, CanvasLayoutMetadataData>([
      ['a', { lane: 'source', rank: 0, stableNodeId: 'a' }],
    ])

    expect(organizeCanvasNodes(nodes, metadata).size).toBe(0)
  })
})
