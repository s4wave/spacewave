import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  areExperimentalCreatorsEnabled,
  setExperimentalCreatorsEnabled,
  useExperimentalCreatorsEnabled,
} from './creator-visibility.js'

afterEach(() => {
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('experimental creator visibility', () => {
  it('enables experimental creators by default in dev builds', () => {
    expect(areExperimentalCreatorsEnabled(true)).toBe(true)
  })

  it('hides experimental creators by default in release builds', () => {
    expect(areExperimentalCreatorsEnabled(false)).toBe(false)
  })

  it('lets release browsers opt in and out with the owned preference', () => {
    setExperimentalCreatorsEnabled(true)

    expect(localStorage.getItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)).toBe('1')
    expect(areExperimentalCreatorsEnabled(false)).toBe(true)

    setExperimentalCreatorsEnabled(false)

    expect(localStorage.getItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)).toBeNull()
    expect(areExperimentalCreatorsEnabled(false)).toBe(false)
  })

  it('updates hook consumers when the owned preference changes', () => {
    const { result } = renderHook(() => useExperimentalCreatorsEnabled(false))

    expect(result.current).toBe(false)

    act(() => setExperimentalCreatorsEnabled(true))

    expect(result.current).toBe(true)

    act(() => setExperimentalCreatorsEnabled(false))

    expect(result.current).toBe(false)
  })
})
