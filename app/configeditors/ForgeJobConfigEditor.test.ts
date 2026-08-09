import { describe, expect, it, vi } from 'vitest'

import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  listObjectsWithType: vi.fn().mockResolvedValue(['cluster/a', 'cluster/b']),
}))

import { loadForgeClusters } from './ForgeJobConfigEditor.js'

describe('loadForgeClusters', () => {
  it('releases each cluster handle before loading the next key', async () => {
    const events: string[] = []
    const world = {
      getObject: vi.fn(async (key: string) => {
        events.push(`get:${key}`)
        return {
          accessWorldState: vi.fn(async () => ({
            unmarshal: vi.fn(async () => ({
              found: true,
              data: Cluster.toBinary({ name: key }),
            })),
            [Symbol.dispose]: () => events.push(`cursor-release:${key}`),
          })),
          release: () => events.push(`object-release:${key}`),
        }
      }),
    } as unknown as IWorldState

    await expect(
      loadForgeClusters(world, new AbortController().signal),
    ).resolves.toEqual([
      { key: 'cluster/a', name: 'cluster/a' },
      { key: 'cluster/b', name: 'cluster/b' },
    ])
    expect(events).toEqual([
      'get:cluster/a',
      'cursor-release:cluster/a',
      'object-release:cluster/a',
      'get:cluster/b',
      'cursor-release:cluster/b',
      'object-release:cluster/b',
    ])
  })
})
