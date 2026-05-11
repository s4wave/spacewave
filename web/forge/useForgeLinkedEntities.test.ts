import { describe, expect, it, vi } from 'vitest'

import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import { loadForgeLinkedEntities } from './useForgeLinkedEntities.js'

describe('loadForgeLinkedEntities', () => {
  it('loads outgoing linked entity types through grouped edge buckets', async () => {
    const signal = new AbortController().signal
    const world = {
      listGraphEdgeBuckets: vi.fn((originObjectKeys: string[]) => {
        if (originObjectKeys[0] === 'forge/dashboard/main') {
          return Promise.resolve({
            buckets: [
              {
                originObjectKey: 'forge/dashboard/main',
                outgoing: [
                  {
                    obj: keyToIRI('forge/job/1'),
                    predicate: '<dashboard-forge-ref>',
                  },
                  {
                    obj: keyToIRI('forge/worker/1'),
                    predicate: '<dashboard-forge-ref>',
                  },
                ],
              },
            ],
          })
        }
        return Promise.resolve({
          buckets: originObjectKeys.map((originObjectKey) => ({
            originObjectKey,
            outgoing: [
              {
                obj: keyToIRI(
                  originObjectKey.startsWith('forge/job') ? 'forge/job' : (
                    'forge/worker'
                  ),
                ),
                predicate: '<type>',
              },
            ],
          })),
        })
      }),
      lookupGraphQuads: vi.fn(() => {
        throw new Error('use listGraphEdgeBuckets')
      }),
    } as unknown as IWorldState

    const entities = await loadForgeLinkedEntities(
      world,
      'forge/dashboard/main',
      '<dashboard-forge-ref>',
      'out',
      signal,
    )

    expect(entities).toEqual([
      { objectKey: 'forge/job/1', typeId: 'forge/job' },
      { objectKey: 'forge/worker/1', typeId: 'forge/worker' },
    ])
    expect(world.lookupGraphQuads).not.toHaveBeenCalled()
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      1,
      ['forge/dashboard/main'],
      200,
      {
        predicate: '<dashboard-forge-ref>',
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      2,
      ['forge/job/1', 'forge/worker/1'],
      1,
      {
        predicate: '<type>',
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      },
    )
  })

  it('loads incoming linked entities from incoming buckets', async () => {
    const signal = new AbortController().signal
    const world = {
      listGraphEdgeBuckets: vi.fn((originObjectKeys: string[]) => {
        if (originObjectKeys[0] === 'forge/worker/1') {
          return Promise.resolve({
            buckets: [
              {
                originObjectKey: 'forge/worker/1',
                incoming: [
                  {
                    subject: keyToIRI('forge/cluster/main'),
                    predicate: '<cluster-to-worker>',
                  },
                ],
              },
            ],
          })
        }
        return Promise.resolve({
          buckets: [
            {
              originObjectKey: 'forge/cluster/main',
              outgoing: [{ obj: keyToIRI('forge/cluster') }],
            },
          ],
        })
      }),
      lookupGraphQuads: vi.fn(() => {
        throw new Error('use listGraphEdgeBuckets')
      }),
    } as unknown as IWorldState

    const entities = await loadForgeLinkedEntities(
      world,
      'forge/worker/1',
      '<cluster-to-worker>',
      'in',
      signal,
    )

    expect(entities).toEqual([
      { objectKey: 'forge/cluster/main', typeId: 'forge/cluster' },
    ])
    expect(world.lookupGraphQuads).not.toHaveBeenCalled()
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      1,
      ['forge/worker/1'],
      200,
      {
        predicate: '<cluster-to-worker>',
        direction: GraphEdgeBucketDirection.IN,
        abortSignal: signal,
      },
    )
  })
})
