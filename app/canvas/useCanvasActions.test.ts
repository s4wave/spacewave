import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, cleanup, act } from '@testing-library/react'

import { useCanvasActions } from './useCanvasActions.js'
import { useCanvasSelection } from './useCanvasSelection.js'
import type { CanvasNodeData, CanvasCallbacks, Viewport } from './types.js'

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

function setup(
  overrides: {
    nodes?: Map<string, CanvasNodeData>
    callbacks?: Partial<CanvasCallbacks>
    viewport?: Viewport
    containerSize?: { width: number; height: number }
  } = {},
) {
  const nodes = overrides.nodes ?? new Map<string, CanvasNodeData>()
  const onNodesChange = vi.fn()
  const onNodesRemove = vi.fn()
  const callbacks: CanvasCallbacks = {
    onNodesChange,
    onNodesRemove,
    ...overrides.callbacks,
  }
  const viewport = overrides.viewport ?? { x: 0, y: 0, scale: 1 }
  const setViewport = vi.fn()
  const containerSize = overrides.containerSize ?? { width: 800, height: 600 }

  const { result } = renderHook(() => {
    const selection = useCanvasSelection()
    const actionsResult = useCanvasActions({
      selection,
      nodes,
      callbacks,
      viewport,
      setViewport,
      containerSize,
    })
    return { selection, ...actionsResult }
  })

  return { result, onNodesChange, onNodesRemove, setViewport }
}

function getViewportCall(
  setViewport: ReturnType<typeof vi.fn>,
  index = 0,
): Viewport {
  return setViewport.mock.calls[index]?.[0] as Viewport
}

describe('useCanvasActions', () => {
  afterEach(() => {
    cleanup()
  })

  it('provides all action functions', () => {
    const { result } = setup()
    const actionKeys = Object.keys(result.current.actions)
    expect(actionKeys).toContain('delete')
    expect(actionKeys).toContain('copy')
    expect(actionKeys).toContain('paste')
    expect(actionKeys).toContain('undo')
    expect(actionKeys).toContain('redo')
    expect(actionKeys).toContain('select-all')
    expect(actionKeys).toContain('deselect')
    expect(actionKeys).toContain('zoom-in')
    expect(actionKeys).toContain('zoom-out')
    expect(actionKeys).toContain('zoom-reset')
    expect(actionKeys).toContain('fit-view')
    expect(actionKeys).toContain('bring-to-front')
    expect(actionKeys).toContain('send-to-back')
  })

  it('exposes moveSelected function', () => {
    const { result } = setup()
    expect(typeof result.current.moveSelected).toBe('function')
  })

  it('delete removes selected nodes via onNodesRemove', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const { result, onNodesRemove } = setup({ nodes })

    // Select node 'a'.
    act(() => {
      result.current.selection.toggleSelect('a', false)
    })

    // Delete selected.
    act(() => {
      result.current.actions.delete()
    })

    expect(onNodesRemove).toHaveBeenCalledWith(['a'])
  })

  it('delete does nothing when nothing is selected', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0 })
    const nodes = new Map([['a', n1]])
    const { result, onNodesRemove } = setup({ nodes })

    act(() => {
      result.current.actions.delete()
    })

    expect(onNodesRemove).not.toHaveBeenCalled()
  })

  it('select-all selects all nodes', () => {
    const n1 = makeNode({ id: 'a' })
    const n2 = makeNode({ id: 'b' })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const { result } = setup({ nodes })

    act(() => {
      result.current.actions['select-all']()
    })

    expect(result.current.selection.selectedNodeIds.has('a')).toBe(true)
    expect(result.current.selection.selectedNodeIds.has('b')).toBe(true)
  })

  it('deselect clears selection', () => {
    const n1 = makeNode({ id: 'a' })
    const nodes = new Map([['a', n1]])
    const { result } = setup({ nodes })

    act(() => {
      result.current.selection.toggleSelect('a', false)
    })
    expect(result.current.selection.selectedNodeIds.size).toBe(1)

    act(() => {
      result.current.actions.deselect()
    })
    expect(result.current.selection.selectedNodeIds.size).toBe(0)
  })

  it('zoom-in increases viewport scale', () => {
    const { result, setViewport } = setup({
      viewport: { x: 0, y: 0, scale: 1 },
    })

    act(() => {
      result.current.actions['zoom-in']()
    })

    expect(setViewport).toHaveBeenCalled()
    const newViewport = getViewportCall(setViewport)
    expect(newViewport.scale).toBeGreaterThan(1)
  })

  it('zoom-out decreases viewport scale', () => {
    const { result, setViewport } = setup({
      viewport: { x: 0, y: 0, scale: 1 },
    })

    act(() => {
      result.current.actions['zoom-out']()
    })

    expect(setViewport).toHaveBeenCalled()
    const newViewport = getViewportCall(setViewport)
    expect(newViewport.scale).toBeLessThan(1)
  })

  it('zoom-reset returns viewport scale to 1', () => {
    const { result, setViewport } = setup({
      viewport: { x: 10, y: 20, scale: 2 },
    })

    act(() => {
      result.current.actions['zoom-reset']()
    })

    expect(setViewport).toHaveBeenCalled()
    const newViewport = getViewportCall(setViewport)
    expect(newViewport.scale).toBe(1)
  })

  it('fit-view resets to center all nodes', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 200, height: 200 })
    const nodes = new Map([['a', n1]])
    const { result, setViewport } = setup({
      nodes,
      containerSize: { width: 800, height: 600 },
    })

    act(() => {
      result.current.actions['fit-view']()
    })

    expect(setViewport).toHaveBeenCalled()
    const vp = getViewportCall(setViewport)
    expect(vp).toHaveProperty('x')
    expect(vp).toHaveProperty('y')
    expect(vp).toHaveProperty('scale')
  })

  it('fit-view resets to origin when no nodes', () => {
    const { result, setViewport } = setup()

    act(() => {
      result.current.actions['fit-view']()
    })

    expect(setViewport).toHaveBeenCalledWith({ x: 0, y: 0, scale: 1 })
  })

  it('bring-to-front sets selected nodes to max zIndex + 1', () => {
    const n1 = makeNode({ id: 'a', zIndex: 0 })
    const n2 = makeNode({ id: 'b', zIndex: 5 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const { result, onNodesChange } = setup({ nodes })

    act(() => {
      result.current.selection.toggleSelect('a', false)
    })

    act(() => {
      result.current.actions['bring-to-front']()
    })

    expect(onNodesChange).toHaveBeenCalled()
    const updated = onNodesChange.mock.calls[0][0] as Map<
      string,
      CanvasNodeData
    >
    // Only the changed node is passed, not the full map.
    expect(updated.size).toBe(1)
    expect(updated.get('a')?.zIndex).toBe(6)
  })

  it('send-to-back sets selected nodes to min zIndex - 1', () => {
    const n1 = makeNode({ id: 'a', zIndex: 3 })
    const n2 = makeNode({ id: 'b', zIndex: 0 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const { result, onNodesChange } = setup({ nodes })

    act(() => {
      result.current.selection.toggleSelect('a', false)
    })

    act(() => {
      result.current.actions['send-to-back']()
    })

    expect(onNodesChange).toHaveBeenCalled()
    const updated = onNodesChange.mock.calls[0][0] as Map<
      string,
      CanvasNodeData
    >
    // Only the changed node is passed, not the full map.
    expect(updated.size).toBe(1)
    expect(updated.get('a')?.zIndex).toBe(-1)
  })

  it('moveSelected moves selected nodes by given offsets', () => {
    const n1 = makeNode({ id: 'a', x: 100, y: 100 })
    const nodes = new Map([['a', n1]])
    const { result, onNodesChange } = setup({ nodes })

    act(() => {
      result.current.selection.toggleSelect('a', false)
    })

    act(() => {
      result.current.moveSelected(10, 0)
    })

    expect(onNodesChange).toHaveBeenCalled()
    const updated = onNodesChange.mock.calls[0][0] as Map<
      string,
      CanvasNodeData
    >
    expect(updated.get('a')?.x).toBe(110)
    expect(updated.get('a')?.y).toBe(100)
  })

  it('moveSelected does nothing when nothing is selected', () => {
    const n1 = makeNode({ id: 'a', x: 100, y: 100 })
    const nodes = new Map([['a', n1]])
    const { result, onNodesChange } = setup({ nodes })

    act(() => {
      result.current.moveSelected(10, 0)
    })

    expect(onNodesChange).not.toHaveBeenCalled()
  })
})
