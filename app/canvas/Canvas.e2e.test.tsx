import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import { useState, useCallback } from 'react'

import '@s4wave/web/style/app.css'

import { Canvas } from './Canvas.js'
import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasCallbacks,
} from './types.js'

// makeNode creates a CanvasNodeData with sensible defaults.
function makeNode(
  overrides: Partial<CanvasNodeData> & { id: string },
): CanvasNodeData {
  return {
    x: 100,
    y: 100,
    width: 200,
    height: 150,
    zIndex: 0,
    type: 'text',
    textContent: 'hello',
    ...overrides,
  }
}

// TestCanvas wraps Canvas with mutable state so mutations are reflected.
function TestCanvas({ initialNodes }: { initialNodes: CanvasNodeData[] }) {
  const [state, setState] = useState<CanvasStateData>(() => ({
    nodes: new Map(initialNodes.map((n) => [n.id, n])),
    edges: [],
    hiddenGraphLinks: [],
  }))

  const callbacks: CanvasCallbacks = {
    onNodesChange: useCallback((changed: Map<string, CanvasNodeData>) => {
      setState((prev) => {
        const next = new Map(prev.nodes)
        for (const [id, n] of changed) next.set(id, n)
        return { ...prev, nodes: next }
      })
    }, []),
    onNodesRemove: useCallback((ids: string[]) => {
      setState((prev) => {
        const next = new Map(prev.nodes)
        for (const id of ids) next.delete(id)
        return { ...prev, nodes: next }
      })
    }, []),
  }

  return (
    <div style={{ width: 800, height: 600 }}>
      <Canvas state={state} callbacks={callbacks} />
    </div>
  )
}

// pointerDrag dispatches a pointerdown, multiple pointermove, and pointerup sequence.
function pointerDrag(
  el: Element,
  from: { x: number; y: number },
  to: { x: number; y: number },
  steps = 5,
) {
  const rect = el.getBoundingClientRect()
  const startX = rect.left + from.x
  const startY = rect.top + from.y
  const endX = rect.left + to.x
  const endY = rect.top + to.y

  el.dispatchEvent(
    new PointerEvent('pointerdown', {
      clientX: startX,
      clientY: startY,
      bubbles: true,
      pointerId: 1,
    }),
  )

  for (let i = 1; i <= steps; i++) {
    const t = i / steps
    el.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: startX + (endX - startX) * t,
        clientY: startY + (endY - startY) * t,
        bubbles: true,
        pointerId: 1,
      }),
    )
  }

  el.dispatchEvent(
    new PointerEvent('pointerup', {
      clientX: endX,
      clientY: endY,
      bubbles: true,
      pointerId: 1,
    }),
  )
}

describe('Canvas node drag does not pan viewport', () => {
  beforeEach(async () => {
    await cleanup()
  })

  it('dragging a node does not change the viewport transform', async () => {
    const nodeA = makeNode({ id: 'a', x: 100, y: 100, width: 200, height: 150 })
    await render(<TestCanvas initialNodes={[nodeA]} />)

    // Wait for node to render.
    const nodeEl = await vi.waitFor(() => {
      const el = document.querySelector('[data-canvas-node="a"]')
      expect(el).toBeTruthy()
      return el!
    })

    // Get the transform container (parent of nodes).
    const viewport = document.querySelector('[data-testid="canvas-viewport"]')!
    const transformDiv = viewport.querySelector('[style*="transform"]')!
    const initialTransform = (transformDiv as HTMLElement).style.transform

    // Drag the node 50px right and 30px down.
    pointerDrag(nodeEl, { x: 100, y: 75 }, { x: 150, y: 105 })

    // Wait a tick for any state updates.
    await new Promise((r) => setTimeout(r, 100))

    // The viewport transform should not have changed.
    const afterTransform = (transformDiv as HTMLElement).style.transform
    expect(afterTransform).toBe(initialTransform)
  })

  it('dragging a node among multiple nodes does not pan', async () => {
    const nodeA = makeNode({ id: 'a', x: 50, y: 50, width: 200, height: 150 })
    const nodeB = makeNode({ id: 'b', x: 300, y: 300, width: 200, height: 150 })
    await render(<TestCanvas initialNodes={[nodeA, nodeB]} />)

    const nodeEl = await vi.waitFor(() => {
      const el = document.querySelector('[data-canvas-node="a"]')
      expect(el).toBeTruthy()
      return el!
    })

    const viewport = document.querySelector('[data-testid="canvas-viewport"]')!
    const transformDiv = viewport.querySelector('[style*="transform"]')!
    const initialTransform = (transformDiv as HTMLElement).style.transform

    // Drag node A a significant distance.
    pointerDrag(nodeEl, { x: 100, y: 75 }, { x: 200, y: 175 }, 10)

    await new Promise((r) => setTimeout(r, 100))

    const afterTransform = (transformDiv as HTMLElement).style.transform
    expect(afterTransform).toBe(initialTransform)
  })
})
