import { cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  Resource,
  UseResourceOptions,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Resource as SDKResource } from '@aptre/bldr-sdk/resource/resource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

const h = vi.hoisted(() => ({
  useResource: vi.fn(),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: h.useResource,
}))

import { useAccessTypedHandle } from './useAccessTypedHandle.js'

class FakeHandle extends SDKResource {}

beforeEach(() => {
  h.useResource.mockReset()
  h.useResource.mockReturnValue(resource<FakeHandle>(null))
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

describe('useAccessTypedHandle released-resource retry contract', () => {
  it('retries typed handles released by either the server or a lost ResourceClient connection', () => {
    const worldState = resource({} as IWorldState)

    renderHook(() =>
      useAccessTypedHandle(worldState, 'world/object-1', FakeHandle),
    )

    const options = h.useResource.mock.calls[0]?.[3] as
      | UseResourceOptions<FakeHandle>
      | undefined
    const retryOnReleasedResource = options?.retryOnReleasedResource

    expect(retryOnReleasedResource).toEqual(expect.any(Object))
    if (
      !retryOnReleasedResource ||
      typeof retryOnReleasedResource !== 'object'
    ) {
      throw new Error('retryOnReleasedResource must be configured as options')
    }

    expect(retryOnReleasedResource.reasons).toEqual([
      'server-released',
      'route-refused',
      'connection-lost',
    ])
  })
})
