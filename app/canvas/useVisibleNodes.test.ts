import { describe, it, expect, afterEach } from 'vitest'
import { renderHook, cleanup } from '@testing-library/react'

import { useVisibleNodes } from './useVisibleNodes.js'
import type { CanvasNodeData, Viewport } from './types.js'
import { VIEWPORT_MARGIN } from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'n1',
    x: 0,
    y: 0,
    width: 200,
    height: 150,
    zIndex: 0,
    type: 'text',
    ...overrides,
  }
}

describe('useVisibleNodes', () => {
  afterEach(() => {
    cleanup()
  })

  it('returns all nodes when they are within viewport', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', x: 100, y: 100, width: 200, height: 150 })],
      ['b', makeNode({ id: 'b', x: 400, y: 200, width: 200, height: 150 })],
    ])
    const viewport: Viewport = { x: 0, y: 0, scale: 1 }
    const containerSize = { width: 1000, height: 800 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.has('a')).toBe(true)
    expect(result.current.has('b')).toBe(true)
  })

  it('excludes nodes far outside the viewport', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', x: 100, y: 100, width: 200, height: 150 })],
      [
        'far',
        makeNode({
          id: 'far',
          x: 10000,
          y: 10000,
          width: 200,
          height: 150,
        }),
      ],
    ])
    const viewport: Viewport = { x: 0, y: 0, scale: 1 }
    const containerSize = { width: 800, height: 600 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.has('a')).toBe(true)
    expect(result.current.has('far')).toBe(false)
  })

  it('includes nodes within VIEWPORT_MARGIN of the visible area', () => {
    const nodes = new Map<string, CanvasNodeData>([
      [
        'margin',
        makeNode({
          id: 'margin',
          // Place just outside the visible area but within the margin.
          x: 800 + VIEWPORT_MARGIN / 2,
          y: 100,
          width: 200,
          height: 150,
        }),
      ],
    ])
    const viewport: Viewport = { x: 0, y: 0, scale: 1 }
    const containerSize = { width: 800, height: 600 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.has('margin')).toBe(true)
  })

  it('accounts for viewport pan offset', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', x: 2000, y: 2000, width: 200, height: 150 })],
    ])
    // Pan the viewport to center on the node.
    const viewport: Viewport = { x: -1800, y: -1800, scale: 1 }
    const containerSize = { width: 800, height: 600 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.has('a')).toBe(true)
  })

  it('accounts for viewport scale', () => {
    const nodes = new Map<string, CanvasNodeData>([
      ['a', makeNode({ id: 'a', x: 3000, y: 3000, width: 200, height: 150 })],
    ])
    // At scale 0.1, the visible area spans 8000x6000 canvas units.
    const viewport: Viewport = { x: 0, y: 0, scale: 0.1 }
    const containerSize = { width: 800, height: 600 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.has('a')).toBe(true)
  })

  it('returns empty set when there are no nodes', () => {
    const nodes = new Map<string, CanvasNodeData>()
    const viewport: Viewport = { x: 0, y: 0, scale: 1 }
    const containerSize = { width: 800, height: 600 }

    const { result } = renderHook(() =>
      useVisibleNodes(nodes, viewport, containerSize),
    )
    expect(result.current.size).toBe(0)
  })
})
