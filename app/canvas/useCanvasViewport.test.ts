import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'

import { useCanvasViewport } from './useCanvasViewport.js'

describe('useCanvasViewport', () => {
  it('initializes with default viewport', () => {
    const { result } = renderHook(() => useCanvasViewport())
    expect(result.current.viewport).toEqual({ x: 0, y: 0, scale: 1 })
  })

  it('setViewport updates viewport state', () => {
    const { result } = renderHook(() => useCanvasViewport())

    act(() => {
      result.current.setViewport({ x: 50, y: 75, scale: 2 })
    })

    expect(result.current.viewport).toEqual({ x: 50, y: 75, scale: 2 })
  })

  it('provides a container ref', () => {
    const { result } = renderHook(() => useCanvasViewport())
    expect(result.current.containerRef).toBeDefined()
    expect(result.current.containerRef.current).toBeNull()
  })

  it('attaches gestures via target ref (no bind function needed)', () => {
    const { result } = renderHook(() => useCanvasViewport())
    // When using target-based gesture binding, there is no bind function.
    // Gestures are attached directly to the containerRef target via effects.
    expect(result.current.containerRef).toBeDefined()
    expect('bind' in result.current).toBe(false)
  })
})
