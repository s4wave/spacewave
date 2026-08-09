import React from 'react'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { CanvasGeometryNode } from './CanvasGeometryNode.js'
import {
  decodeCanvasClipboardPaste,
  encodeCanvasClipboard,
  isValidCanvasClipboardNode,
} from './clipboard.js'
import type { CanvasNodeData } from './types.js'

function drawingNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'drawing',
    x: 1,
    y: 2,
    width: 100,
    height: 80,
    zIndex: 0,
    type: 'drawing',
    geometry: {
      kind: 'pen',
      color: '#abcdef',
      points: [
        { x: 1.25, y: 2.5 },
        { x: 9.75, y: 10.5 },
      ],
    },
    ...overrides,
  }
}

function decode(nodes: CanvasNodeData[], copiedAt = 1234) {
  return decodeCanvasClipboardPaste(encodeCanvasClipboard(nodes), copiedAt)
}

describe('Canvas clipboard boundary', () => {
  afterEach(cleanup)

  it('translates and round-trips ordinary geometry through generated binary bytes', () => {
    const node = drawingNode()
    const decoded = decode([node])
    expect(decoded).toEqual(
      new Map([
        [
          'drawing-copy-1234-0',
          { ...node, id: 'drawing-copy-1234-0', x: 21, y: 22 },
        ],
      ]),
    )
    render(<CanvasGeometryNode node={[...decoded!.values()][0]!} />)
    expect(document.querySelector('path')?.getAttribute('stroke')).toBe(
      '#abcdef',
    )
  })

  it.each(['', '[]', 'spacewave-canvas:v1:not base64', 'spacewave-canvas:v1:'])(
    'rejects malformed or untrusted payload %j',
    (payload) => {
      expect(decodeCanvasClipboardPaste(payload, 1234)).toBeNull()
    },
  )

  it.each([
    ['translated x coordinate', { x: 999_999_990 }],
    ['translated y coordinate', { y: 999_999_990 }],
    ['1024-byte source ID plus paste suffix', { id: 'x'.repeat(1024) }],
    ['multibyte source ID plus paste suffix', { id: 'é'.repeat(507) }],
    ['multibyte text bytes', { textContent: 'é'.repeat(512 * 1024 + 1) }],
    ['multibyte object-key bytes', { objectKey: 'é'.repeat(2049) }],
    ['multibyte view-path bytes', { viewPath: 'é'.repeat(2049) }],
    ['width', { width: 1_000_001 }],
    ['height', { height: Number.POSITIVE_INFINITY }],
  ] satisfies [string, Partial<CanvasNodeData>][])(
    'rejects unsafe %s',
    (_name, overrides) => {
      expect(decode([drawingNode(overrides)])).toBeNull()
    },
  )

  it.each([
    [
      'multibyte color',
      (node: CanvasNodeData) => (node.geometry!.color = '#éééééé'),
    ],
    [
      'geometry coordinate',
      (node: CanvasNodeData) => (node.geometry!.points[0]!.x = 1_000_000_001),
    ],
    [
      'geometry point count',
      (node: CanvasNodeData) => (node.geometry!.points = [{ x: 0, y: 0 }]),
    ],
  ])('rejects unsafe %s', (_name, mutate) => {
    const node = drawingNode()
    mutate(node)
    expect(decode([node])).toBeNull()
  })
})

describe('isValidCanvasClipboardNode', () => {
  it('uses UTF-8 byte counts shared by decoded and translated nodes', () => {
    expect(
      isValidCanvasClipboardNode(
        drawingNode({
          id: 'é'.repeat(512),
          textContent: 'é'.repeat(512 * 1024),
        }),
      ),
    ).toBe(true)
    expect(
      isValidCanvasClipboardNode(
        drawingNode({ id: 'é'.repeat(513), textContent: 'ordinary' }),
      ),
    ).toBe(false)
    expect(
      isValidCanvasClipboardNode(
        drawingNode({ textContent: 'é'.repeat(512 * 1024 + 1) }),
      ),
    ).toBe(false)
  })

  it('matches durable position, dimension, path, color, and geometry bounds', () => {
    expect(isValidCanvasClipboardNode(drawingNode())).toBe(true)
    expect(isValidCanvasClipboardNode(drawingNode({ x: Number.NaN }))).toBe(
      false,
    )
    expect(isValidCanvasClipboardNode(drawingNode({ width: 0 }))).toBe(false)
    expect(
      isValidCanvasClipboardNode(drawingNode({ objectKey: 'é'.repeat(2049) })),
    ).toBe(false)
    const invalidColor = drawingNode()
    invalidColor.geometry!.color = 'red'
    expect(isValidCanvasClipboardNode(invalidColor)).toBe(false)
    const invalidPoint = drawingNode()
    invalidPoint.geometry!.points[0]!.y = Number.NEGATIVE_INFINITY
    expect(isValidCanvasClipboardNode(invalidPoint)).toBe(false)
  })
})
