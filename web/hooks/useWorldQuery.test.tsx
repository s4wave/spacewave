import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { WorldQuerySnapshot } from '@s4wave/sdk/world/world-query.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

const h = vi.hoisted(() => ({
  push: undefined as
    | ((snapshot: WorldQuerySnapshot<string>) => void)
    | undefined,
}))

// The live watch yields each snapshot the test pushes.
vi.mock('@s4wave/sdk/world/world-query.js', () => ({
  watchWorldQuery: async function* () {
    for (;;) {
      yield await new Promise<WorldQuerySnapshot<string>>((resolve) => {
        h.push = resolve
      })
    }
  },
}))

import { useWorldQuery } from './useWorldQuery.js'

function resource(value: IWorldState): Resource<IWorldState> {
  return { value, loading: false, error: null, retry: vi.fn() }
}

afterEach(() => {
  cleanup()
  h.push = undefined
})

describe('useWorldQuery', () => {
  it('reads a World without an Engine once', async () => {
    const world = resource({} as IWorldState)
    const query = vi.fn(async () => 'snapshot')
    const { result } = renderHook(() => useWorldQuery(world, query, []))

    await waitFor(() => expect(result.current.value).toBe('snapshot'))
    expect(query).toHaveBeenCalledOnce()
    expect(query).toHaveBeenCalledWith(world.value, expect.any(AbortSignal))
  })

  it('follows each revision of a live World', async () => {
    const world = resource({ getEngine: () => ({}) } as unknown as IWorldState)
    const { result } = renderHook(() =>
      useWorldQuery(world, async () => 'unused', []),
    )

    await waitFor(() => expect(h.push).toBeDefined())
    act(() => h.push?.({ seqno: 1n, value: 'first' }))
    await waitFor(() => expect(result.current.value).toBe('first'))
    act(() => h.push?.({ seqno: 2n, value: 'second' }))
    await waitFor(() => expect(result.current.value).toBe('second'))
  })
})
