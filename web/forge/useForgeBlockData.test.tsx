import { cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'

const h = vi.hoisted(() => ({
  useResource: vi.fn(),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: h.useResource,
}))

import { useForgeBlockData } from './useForgeBlockData.js'

const dashboardBlockTypeID = 'spacewave/forge/dashboard'

beforeEach(() => {
  h.useResource.mockReset()
  h.useResource.mockReturnValue(resource(null))
})

afterEach(() => {
  cleanup()
})

function resource<T>(value: T | null): Resource<T> {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

describe('useForgeBlockData', () => {
  it('uses the registered block type before decoding the generated message', async () => {
    const signal = new AbortController().signal
    const dashboardData = ForgeDashboard.toBinary({
      name: 'Forge',
      createdAt: new Date('2026-07-10T00:00:00Z'),
    })
    const typeFragment = new TextEncoder().encode('types/unixfs/fs-node')
    const wrongMessageData = new Uint8Array([
      0x0a,
      typeFragment.length,
      ...typeFragment,
    ])
    const unmarshal = vi.fn((req: { blockType?: string }) => ({
      found: true,
      data:
        req.blockType === dashboardBlockTypeID
          ? dashboardData
          : wrongMessageData,
    }))
    const dispose = vi.fn()
    const objectState = {
      accessWorldState: vi.fn().mockResolvedValue({
        unmarshal,
        [Symbol.dispose]: dispose,
      }),
    } as unknown as IObjectState

    renderHook(() =>
      useForgeBlockData(objectState, dashboardBlockTypeID, ForgeDashboard),
    )

    const load = h.useResource.mock.calls[0]?.[0] as (
      signal: AbortSignal,
    ) => Promise<unknown>
    const result = await load(signal)

    expect(unmarshal).toHaveBeenCalledWith(
      { blockType: dashboardBlockTypeID },
      signal,
    )
    expect(result).toEqual(expect.objectContaining({ name: 'Forge' }))
    expect(dispose).toHaveBeenCalledOnce()
  })
})
