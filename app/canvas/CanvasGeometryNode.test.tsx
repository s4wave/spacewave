import React from 'react'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { CanvasGeometryNode } from './CanvasGeometryNode.js'
import type { CanvasGeometryKind } from './geometry.js'
import type { CanvasNodeData } from './types.js'

function makeNode(kind: CanvasGeometryKind): CanvasNodeData {
  return {
    id: `shape-${kind}`,
    x: 0,
    y: 0,
    width: 120,
    height: 80,
    zIndex: 0,
    type: kind === 'pen' ? 'drawing' : 'shape',
    geometry: {
      kind,
      color: '#16a34a',
      points: [
        { x: 8, y: 8 },
        { x: 112, y: 72 },
      ],
    },
  }
}

describe('CanvasGeometryNode', () => {
  afterEach(cleanup)

  it.each([
    ['line', 'line'],
    ['arrow', 'line'],
    ['rectangle', 'rect'],
    ['ellipse', 'ellipse'],
  ] as const)('renders %s in the shared shape node model', (kind, element) => {
    render(<CanvasGeometryNode node={makeNode(kind)} />)

    const svg = document.querySelector(`[data-canvas-geometry="${kind}"]`)
    expect(svg).toBeTruthy()
    expect(svg?.querySelector(element)?.getAttribute('stroke')).toBe('#16a34a')
  })
})
