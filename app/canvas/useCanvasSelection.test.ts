import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'

import { useCanvasSelection } from './useCanvasSelection.js'
import type { CanvasNodeData } from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'node-1',
    x: 100,
    y: 100,
    width: 200,
    height: 150,
    zIndex: 0,
    type: 'text',
    ...overrides,
  }
}

describe('useCanvasSelection', () => {
  it('starts with empty selection', () => {
    const { result } = renderHook(() => useCanvasSelection())
    expect(result.current.selectedNodeIds.size).toBe(0)
    expect(result.current.dragRect).toBeNull()
  })

  it('toggleSelect adds a node to selection', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(true)
    expect(result.current.selectedNodeIds.size).toBe(1)
  })

  it('toggleSelect without shift replaces selection', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })
    act(() => {
      result.current.toggleSelect('node-2', false)
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(false)
    expect(result.current.selectedNodeIds.has('node-2')).toBe(true)
    expect(result.current.selectedNodeIds.size).toBe(1)
  })

  it('toggleSelect with shift adds to selection', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })
    act(() => {
      result.current.toggleSelect('node-2', true)
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(true)
    expect(result.current.selectedNodeIds.has('node-2')).toBe(true)
    expect(result.current.selectedNodeIds.size).toBe(2)
  })

  it('toggleSelect with shift deselects if already selected', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })
    act(() => {
      result.current.toggleSelect('node-1', true)
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(false)
    expect(result.current.selectedNodeIds.size).toBe(0)
  })

  it('clearSelection empties the set', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })
    act(() => {
      result.current.clearSelection()
    })

    expect(result.current.selectedNodeIds.size).toBe(0)
  })

  it('selectAll selects all nodes', () => {
    const { result } = renderHook(() => useCanvasSelection())
    const nodes = new Map([
      ['a', makeNode({ id: 'a' })],
      ['b', makeNode({ id: 'b' })],
      ['c', makeNode({ id: 'c' })],
    ])

    act(() => {
      result.current.selectAll(nodes)
    })

    expect(result.current.selectedNodeIds.size).toBe(3)
    expect(result.current.selectedNodeIds.has('a')).toBe(true)
    expect(result.current.selectedNodeIds.has('b')).toBe(true)
    expect(result.current.selectedNodeIds.has('c')).toBe(true)
  })

  it('setSelection replaces with given set', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })
    act(() => {
      result.current.setSelection(new Set(['x', 'y']))
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(false)
    expect(result.current.selectedNodeIds.has('x')).toBe(true)
    expect(result.current.selectedNodeIds.has('y')).toBe(true)
  })

  it('drag rect lifecycle: start, update, end selects nodes', () => {
    const { result } = renderHook(() => useCanvasSelection())

    // Nodes at known positions.
    const nodes = new Map([
      [
        'inside',
        makeNode({ id: 'inside', x: 50, y: 50, width: 100, height: 100 }),
      ],
      [
        'outside',
        makeNode({ id: 'outside', x: 500, y: 500, width: 100, height: 100 }),
      ],
    ])

    // Start drag rect at screen coords with viewport at origin, scale 1.
    act(() => {
      result.current.startDragRect(0, 0)
    })
    expect(result.current.dragRect).toBeTruthy()

    // Update drag rect to cover the "inside" node.
    act(() => {
      result.current.updateDragRect(200, 200)
    })
    expect(result.current.dragRect?.endX).toBe(200)
    expect(result.current.dragRect?.endY).toBe(200)

    // End drag rect - should select "inside" but not "outside".
    act(() => {
      result.current.endDragRect(nodes, 0, 0, 1)
    })

    expect(result.current.dragRect).toBeNull()
    expect(result.current.selectedNodeIds.has('inside')).toBe(true)
    expect(result.current.selectedNodeIds.has('outside')).toBe(false)
  })

  it('drag rect accounts for viewport offset and scale', () => {
    const { result } = renderHook(() => useCanvasSelection())

    // Node at canvas position (200, 200).
    const nodes = new Map([
      ['n1', makeNode({ id: 'n1', x: 200, y: 200, width: 50, height: 50 })],
    ])

    // Viewport offset: x=100, y=100, scale=2.
    // Screen rect (0,0)-(250,250) maps to canvas rect:
    // left = (0 - 100) / 2 = -50, top = -50
    // right = (250 - 100) / 2 = 75, bottom = 75
    // Node at (200, 200) is outside this range.
    act(() => {
      result.current.startDragRect(0, 0)
    })
    act(() => {
      result.current.updateDragRect(250, 250)
    })
    act(() => {
      result.current.endDragRect(nodes, 100, 100, 2)
    })

    expect(result.current.selectedNodeIds.has('n1')).toBe(false)

    // Now with a rect that covers (200, 200) in canvas coords.
    // Canvas (200, 200) at scale=2, offset=(100,100): screen = 200*2+100 = 500
    act(() => {
      result.current.startDragRect(400, 400)
    })
    act(() => {
      result.current.updateDragRect(600, 600)
    })
    act(() => {
      result.current.endDragRect(nodes, 100, 100, 2)
    })

    expect(result.current.selectedNodeIds.has('n1')).toBe(true)
  })

  it('toggleSelect sets focus to border', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.toggleSelect('node-1', false)
    })

    expect(result.current.focus).toBe('border')
  })

  it('selectWithFocus sets focus to the given mode', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.selectWithFocus('node-1', false, 'content')
    })

    expect(result.current.selectedNodeIds.has('node-1')).toBe(true)
    expect(result.current.focus).toBe('content')

    act(() => {
      result.current.selectWithFocus('node-1', false, 'border')
    })

    expect(result.current.focus).toBe('border')
  })

  it('setFocus changes focus mode without changing selection', () => {
    const { result } = renderHook(() => useCanvasSelection())

    act(() => {
      result.current.selectWithFocus('node-1', false, 'content')
    })
    expect(result.current.focus).toBe('content')

    act(() => {
      result.current.setFocus('border')
    })
    expect(result.current.focus).toBe('border')
    expect(result.current.selectedNodeIds.has('node-1')).toBe(true)
  })
})
