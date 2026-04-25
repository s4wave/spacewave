import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup, fireEvent } from '@testing-library/react'

import { CanvasMinimap } from './CanvasMinimap.js'
import type { CanvasNodeData, Viewport } from './types.js'

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

const defaultViewport: Viewport = { x: 0, y: 0, scale: 1 }
const defaultContainerSize = { width: 800, height: 600 }

describe('CanvasMinimap', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders a minimap container with fixed dimensions', () => {
    render(
      <CanvasMinimap
        nodes={new Map()}
        viewport={defaultViewport}
        containerSize={defaultContainerSize}
        onViewportChange={vi.fn()}
      />,
    )
    const minimap = document.querySelector(
      '[style*="width: 200px"]',
    ) as HTMLElement
    expect(minimap).toBeTruthy()
    expect(minimap.style.height).toBe('150px')
  })

  it('renders node rectangles in the minimap', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 200, height: 100 })
    const n2 = makeNode({ id: 'b', x: 400, y: 300, width: 200, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    render(
      <CanvasMinimap
        nodes={nodes}
        viewport={defaultViewport}
        containerSize={defaultContainerSize}
        onViewportChange={vi.fn()}
      />,
    )
    // Node rects have bg-foreground-alt/25 class.
    const rects = document.querySelectorAll('.bg-foreground-alt\\/25')
    expect(rects.length).toBe(2)
  })

  it('renders a viewport indicator rectangle', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0 })
    const nodes = new Map([['a', n1]])
    render(
      <CanvasMinimap
        nodes={nodes}
        viewport={defaultViewport}
        containerSize={defaultContainerSize}
        onViewportChange={vi.fn()}
      />,
    )
    // Viewport indicator has border-brand/30 class.
    const vpIndicator = document.querySelector('.border-brand\\/30')
    expect(vpIndicator).toBeTruthy()
  })

  it('calls onViewportChange when clicked', () => {
    const onViewportChange = vi.fn()
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 200, height: 200 })
    const nodes = new Map([['a', n1]])
    render(
      <CanvasMinimap
        nodes={nodes}
        viewport={defaultViewport}
        containerSize={defaultContainerSize}
        onViewportChange={onViewportChange}
      />,
    )
    const minimap = document.querySelector(
      '[style*="width: 200px"]',
    ) as HTMLElement
    // We need to mock getBoundingClientRect for the click handler.
    minimap.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      right: 200,
      bottom: 150,
      width: 200,
      height: 150,
      x: 0,
      y: 0,
      toJSON: () => {},
    })
    fireEvent.click(minimap, { clientX: 100, clientY: 75 })
    expect(onViewportChange).toHaveBeenCalled()
    const call = onViewportChange.mock.calls[0][0]
    expect(call).toHaveProperty('x')
    expect(call).toHaveProperty('y')
    expect(call).toHaveProperty('scale')
  })

  it('renders no node rects when nodes are empty', () => {
    render(
      <CanvasMinimap
        nodes={new Map()}
        viewport={defaultViewport}
        containerSize={defaultContainerSize}
        onViewportChange={vi.fn()}
      />,
    )
    const rects = document.querySelectorAll('.bg-foreground-alt\\/25')
    expect(rects.length).toBe(0)
  })

  it('preserves viewport scale on click navigation', () => {
    const onViewportChange = vi.fn()
    const viewport: Viewport = { x: 0, y: 0, scale: 2 }
    const n1 = makeNode({ id: 'a', x: 0, y: 0 })
    const nodes = new Map([['a', n1]])
    render(
      <CanvasMinimap
        nodes={nodes}
        viewport={viewport}
        containerSize={defaultContainerSize}
        onViewportChange={onViewportChange}
      />,
    )
    const minimap = document.querySelector(
      '[style*="width: 200px"]',
    ) as HTMLElement
    minimap.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      right: 200,
      bottom: 150,
      width: 200,
      height: 150,
      x: 0,
      y: 0,
      toJSON: () => {},
    })
    fireEvent.click(minimap, { clientX: 50, clientY: 50 })
    expect(onViewportChange).toHaveBeenCalled()
    // Scale should be preserved from the current viewport.
    expect(onViewportChange.mock.calls[0][0].scale).toBe(2)
  })
})
