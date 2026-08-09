import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Canvas } from './Canvas.js'
import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasCallbacks,
  CanvasEdgeData,
} from './types.js'

vi.mock('./CanvasContextMenu.js', () => ({
  CanvasContextMenu: ({
    state,
  }: {
    state: { position: { x: number; y: number } } | null
  }) =>
    state ? (
      <div
        data-testid="canvas-context-menu"
        data-x={state.position.x}
        data-y={state.position.y}
      />
    ) : null,
}))

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'node-1',
    x: 100,
    y: 100,
    width: 200,
    height: 150,
    zIndex: 0,
    type: 'text',
    textContent: 'Hello world',
    ...overrides,
  }
}

function makeState(
  nodes: CanvasNodeData[] = [],
  edges: CanvasEdgeData[] = [],
): CanvasStateData {
  const nodeMap = new Map<string, CanvasNodeData>()
  for (const n of nodes) {
    nodeMap.set(n.id, n)
  }
  return {
    nodes: nodeMap,
    edges,
    hiddenGraphLinks: [],
    layoutMetadata: new Map(),
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

describe('Canvas', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders without crashing with empty state', () => {
    const state = makeState()
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)
    const root = document.querySelector('[tabindex="0"]')
    expect(root).toBeTruthy()
  })

  it('renders nodes at correct positions', () => {
    const node = makeNode({
      id: 'pos-test',
      x: 50,
      y: 75,
      width: 120,
      height: 80,
    })
    const state = makeState([node])
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const el = document.querySelector(
      '[data-canvas-node="pos-test"]',
    ) as HTMLElement
    expect(el).toBeTruthy()
    expect(el.style.left).toBe('50px')
    expect(el.style.top).toBe('75px')
    expect(el.style.width).toBe('120px')
    expect(el.style.height).toBe('80px')
  })

  it('renders text node content', () => {
    const node = makeNode({ textContent: 'Test content here' })
    const state = makeState([node])
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    expect(screen.getByText('Test content here')).toBeTruthy()
  })

  it('toolbar renders tool buttons', () => {
    const state = makeState()
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    expect(screen.getByLabelText('Select (V)')).toBeTruthy()
    expect(screen.getByLabelText('Draw (D)')).toBeTruthy()
    expect(screen.getByLabelText('Text (T)')).toBeTruthy()
    expect(screen.getByLabelText('Object (O)')).toBeTruthy()
    expect(screen.getByLabelText('Zoom In (+)')).toBeTruthy()
    expect(screen.getByLabelText('Zoom Out (-)')).toBeTruthy()
    expect(screen.getByLabelText('Fit View')).toBeTruthy()
  })

  it('node selection triggers onNodeSelect callback', async () => {
    const user = userEvent.setup()
    const node = makeNode({ id: 'sel-test' })
    const state = makeState([node])
    const onNodeSelect = vi.fn<(nodeIds: Set<string>) => void>()
    const callbacks = makeCallbacks({ onNodeSelect })
    render(<Canvas state={state} callbacks={callbacks} />)

    const el = document.querySelector(
      '[data-canvas-node="sel-test"]',
    ) as HTMLElement
    expect(el).toBeTruthy()
    await user.click(el)

    expect(onNodeSelect).toHaveBeenCalled()
    const lastCall = onNodeSelect.mock.lastCall
    expect(lastCall).toBeDefined()
    const selected = lastCall?.[0] ?? new Set<string>()
    expect(selected.has('sel-test')).toBe(true)
  })

  it('renders edge layer SVG', () => {
    const node1 = makeNode({ id: 'e-n1', x: 0, y: 0, width: 100, height: 100 })
    const node2 = makeNode({
      id: 'e-n2',
      x: 300,
      y: 300,
      width: 100,
      height: 100,
    })
    const edge: CanvasEdgeData = {
      id: 'edge-1',
      sourceNodeId: 'e-n1',
      targetNodeId: 'e-n2',
      style: 'bezier',
    }
    const state = makeState([node1, node2], [edge])
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const svg = document.querySelector('svg')
    expect(svg).toBeTruthy()

    const paths = svg?.querySelectorAll('path')
    expect(paths && paths.length > 0).toBe(true)
  })

  it('minimap renders', () => {
    const node = makeNode()
    const state = makeState([node])
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const minimaps = document.querySelectorAll('[style*="width: 200px"]')
    expect(minimaps.length).toBeGreaterThan(0)
  })

  it('clicking background clears selection', async () => {
    const user = userEvent.setup()
    const node = makeNode({ id: 'esc-test' })
    const state = makeState([node])
    const onNodeSelect = vi.fn()
    const callbacks = makeCallbacks({ onNodeSelect })
    render(<Canvas state={state} callbacks={callbacks} />)

    const el = document.querySelector(
      '[data-canvas-node="esc-test"]',
    ) as HTMLElement
    await user.click(el)

    const root = document.querySelector('[tabindex="0"]') as HTMLElement
    await user.click(root)

    const lastCall = onNodeSelect.mock.calls[onNodeSelect.mock.calls.length - 1]
    const selected = lastCall[0] as Set<string>
    expect(selected.size).toBe(0)
  })

  it('focusNodeId selects and centers the target node', async () => {
    const node = makeNode({ id: 'focus-test', x: 200, y: 100 })
    const state = makeState([node])
    const onNodeSelect = vi.fn<(nodeIds: Set<string>) => void>()
    const callbacks = makeCallbacks({ onNodeSelect })
    render(
      <Canvas state={state} callbacks={callbacks} focusNodeId="focus-test" />,
    )

    const calls = onNodeSelect.mock.calls as [Set<string>][]
    await waitFor(() => {
      const call = calls.find(([selectedIds]) => selectedIds.has('focus-test'))
      expect(call).toBeTruthy()
    })
    const viewport = screen.getByTestId('canvas-viewport')
    const transformDiv = viewport.querySelector('[style*="transform"]')
    expect(transformDiv?.getAttribute('style')).toContain(
      'translate3d(-300px, -175px, 0) scale(1)',
    )
  })

  it('passes the background context menu the clicked screen position', async () => {
    const state = makeState()
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const viewport = screen.getByTestId('canvas-viewport')
    fireEvent.contextMenu(viewport, { clientX: 180, clientY: 220 })

    const menu = await waitFor(() => screen.getByTestId('canvas-context-menu'))
    expect(menu.getAttribute('data-x')).toBe('180')
    expect(menu.getAttribute('data-y')).toBe('220')
  })

  it('renders multiple nodes', () => {
    const nodes = [
      makeNode({ id: 'multi-1', x: 0, y: 0 }),
      makeNode({ id: 'multi-2', x: 300, y: 0 }),
      makeNode({ id: 'multi-3', x: 0, y: 300 }),
    ]
    const state = makeState(nodes)
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    expect(document.querySelector('[data-canvas-node="multi-1"]')).toBeTruthy()
    expect(document.querySelector('[data-canvas-node="multi-2"]')).toBeTruthy()
    expect(document.querySelector('[data-canvas-node="multi-3"]')).toBeTruthy()
  })

  it('all nodes have touch-action none', () => {
    const nodes = [
      makeNode({ id: 'ta-1', x: 0, y: 0 }),
      makeNode({ id: 'ta-2', x: 300, y: 0 }),
    ]
    const state = makeState(nodes)
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const el1 = document.querySelector(
      '[data-canvas-node="ta-1"]',
    ) as HTMLElement
    const el2 = document.querySelector(
      '[data-canvas-node="ta-2"]',
    ) as HTMLElement
    expect(el1.style.touchAction).toBe('none')
    expect(el2.style.touchAction).toBe('none')
  })

  it('does not fire onNodesChange during drag, only on drop', () => {
    const node = makeNode({ id: 'drag-test', x: 100, y: 100 })
    const state = makeState([node])
    const onNodesChange = vi.fn()
    const callbacks = makeCallbacks({ onNodesChange })
    render(<Canvas state={state} callbacks={callbacks} />)

    const el = document.querySelector(
      '[data-canvas-node="drag-test"]',
    ) as HTMLElement

    fireEvent.pointerDown(el, { clientX: 150, clientY: 150, pointerId: 1 })
    fireEvent.pointerMove(el, { clientX: 170, clientY: 160, pointerId: 1 })
    fireEvent.pointerMove(el, { clientX: 190, clientY: 170, pointerId: 1 })

    expect(onNodesChange).not.toHaveBeenCalled()

    fireEvent.pointerUp(el, { clientX: 190, clientY: 170, pointerId: 1 })
  })

  it('viewport transform is applied to the content layer', () => {
    const state = makeState()
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const transformDiv = document.querySelector(
      '[style*="translate3d(0px, 0px, 0) scale(1)"]',
    )
    expect(transformDiv).toBeTruthy()
  })

  it('viewport container has overflow hidden and touch-none', () => {
    const state = makeState()
    const callbacks = makeCallbacks()
    render(<Canvas state={state} callbacks={callbacks} />)

    const container = document.querySelector('.overflow-hidden.touch-none')
    expect(container).toBeTruthy()
  })
})
