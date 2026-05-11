import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useRenderDelay } from './useRenderDelay.js'

describe('useRenderDelay', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('stays false until the delay elapses', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useRenderDelay(250))

    expect(result.current).toBe(false)
    void act(() => vi.advanceTimersByTime(249))
    expect(result.current).toBe(false)
    void act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe(true)
  })

  it('resets when the delay changes', async () => {
    vi.useFakeTimers()
    const { result, rerender } = renderHook(
      ({ ms }: { ms: number }) => useRenderDelay(ms),
      { initialProps: { ms: 100 } },
    )

    void act(() => vi.advanceTimersByTime(100))
    expect(result.current).toBe(true)

    rerender({ ms: 200 })
    await act(async () => {})
    expect(result.current).toBe(false)

    void act(() => vi.advanceTimersByTime(199))
    expect(result.current).toBe(false)
    void act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe(true)
  })
})
