import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup, act, fireEvent } from '@testing-library/react'

import { CanvasNode } from './CanvasNode.js'
import type { CanvasNodeData, CanvasCallbacks } from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'node-1',
    x: 100,
    y: 200,
    width: 300,
    height: 250,
    zIndex: 0,
    type: 'text',
    textContent: 'Hello',
    ...overrides,
  }
}

function makeCallbacks(
  overrides: Partial<CanvasCallbacks> = {},
): CanvasCallbacks {
  return {
    onNodesChange: vi.fn(),
    onEdgesChange: vi.fn(),
    onNodeSelect: vi.fn(),
    ...overrides,
  }
}

describe('CanvasNode', () => {
  afterEach(() => {
    cleanup()
  })

  it('positions node with correct style properties', () => {
    const node = makeNode({
      id: 'style-test',
      x: 50,
      y: 75,
      width: 120,
      height: 80,
      zIndex: 5,
    })
    render(
      <CanvasNode
        node={node}
        scale={1}
        selected={false}
        visible={true}
        callbacks={makeCallbacks()}
        focus="border"
        onSelectWithFocus={vi.fn()}
        onMove={vi.fn()}
        onMoveEnd={vi.fn()}
      />,
    )

    const el = document.querySelector(
      '[data-canvas-node="style-test"]',
    ) as HTMLElement
    expect(el.style.getPropertyValue('--canvas-node-left')).toBe('50px')
    expect(el.style.getPropertyValue('--canvas-node-top')).toBe('75px')
    expect(el.style.getPropertyValue('--canvas-node-width')).toBe('120px')
    expect(el.style.getPropertyValue('--canvas-node-height')).toBe('80px')
  })

  it('returns null when not visible and after unmount debounce', () => {
    vi.useFakeTimers()
    const node = makeNode({ id: 'unmount-test' })
    const { rerender, container } = render(
      <CanvasNode
        node={node}
        scale={1}
        selected={false}
        visible={true}
        callbacks={makeCallbacks()}
        focus="border"
        onSelectWithFocus={vi.fn()}
        onMove={vi.fn()}
        onMoveEnd={vi.fn()}
      />,
    )

    expect(
      container.querySelector('[data-canvas-node="unmount-test"]'),
    ).toBeTruthy()

    // Set visible=false and wait for debounce.
    rerender(
      <CanvasNode
        node={node}
        scale={1}
        selected={false}
        visible={false}
        callbacks={makeCallbacks()}
        focus="border"
        onSelectWithFocus={vi.fn()}
        onMove={vi.fn()}
        onMoveEnd={vi.fn()}
      />,
    )

    // Still mounted during debounce period.
    expect(
      container.querySelector('[data-canvas-node="unmount-test"]'),
    ).toBeTruthy()

    // Advance past debounce.
    act(() => {
      vi.advanceTimersByTime(3000)
    })

    expect(
      container.querySelector('[data-canvas-node="unmount-test"]'),
    ).toBeFalsy()

    vi.useRealTimers()
  })

  it('renders edge drag handles for world-object nodes', () => {
    const node = makeNode({
      id: 'world-object-test',
      type: 'world_object',
      objectKey: 'object-1',
      textContent: undefined,
    })
    render(
      <CanvasNode
        node={node}
        scale={1}
        selected={false}
        visible={true}
        callbacks={makeCallbacks()}
        focus="border"
        onSelectWithFocus={vi.fn()}
        onMove={vi.fn()}
        onMoveEnd={vi.fn()}
      />,
    )

    const el = document.querySelector(
      '[data-canvas-node="world-object-test"]',
    ) as HTMLElement
    expect(el.querySelectorAll('[data-drag-handle]').length).toBe(4)
  })

  it('treats edge drag handles as border clicks', () => {
    const node = makeNode({
      id: 'world-object-click',
      type: 'world_object',
      objectKey: 'object-2',
      textContent: undefined,
    })
    const onSelectWithFocus = vi.fn()
    render(
      <CanvasNode
        node={node}
        scale={1}
        selected={false}
        visible={true}
        callbacks={makeCallbacks()}
        focus="border"
        onSelectWithFocus={onSelectWithFocus}
        onMove={vi.fn()}
        onMoveEnd={vi.fn()}
      />,
    )

    const handle = document.querySelector(
      '[data-canvas-node="world-object-click"] [data-drag-handle]',
    ) as HTMLElement
    expect(handle).toBeTruthy()

    fireEvent.click(handle)

    expect(onSelectWithFocus).toHaveBeenCalledWith(
      'world-object-click',
      false,
      'border',
    )
  })
})
