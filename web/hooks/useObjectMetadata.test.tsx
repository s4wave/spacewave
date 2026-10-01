import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ListObjectsRequest } from '@s4wave/sdk/world/world.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useObjectMetadata } from './useObjectMetadata.js'

// snapshotWorld is a World without an Engine whose listing holds keys.
function snapshotWorld(keys: string[]): Resource<IWorldState> {
  const listObjects = vi.fn(async ({ prefix = '' }: ListObjectsRequest) => ({
    objects: keys
      .filter((key) => key.startsWith(prefix))
      .slice(0, 1)
      .map((key) => ({ objectKey: key, typeId: `type/${key}` })),
  }))
  return {
    value: { listObjects } as unknown as IWorldState,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

afterEach(cleanup)

describe('useObjectMetadata', () => {
  it('returns the metadata of an existing key', async () => {
    const world = snapshotWorld(['docs', 'docs/a'])
    const { result } = renderHook(() => useObjectMetadata(world, 'docs'))

    await waitFor(() =>
      expect(result.current.value).toEqual({
        objectKey: 'docs',
        typeId: 'type/docs',
      }),
    )
  })

  it('returns null when only longer keys share the prefix', async () => {
    const world = snapshotWorld(['docs/a'])
    const { result } = renderHook(() => useObjectMetadata(world, 'docs'))

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.value).toBeNull()
  })
})
