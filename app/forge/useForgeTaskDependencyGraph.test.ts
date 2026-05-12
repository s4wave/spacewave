import { describe, expect, it, vi } from 'vitest'

import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import {
  PRED_TASK_TO_CACHED,
  PRED_TASK_TO_SUBTASK,
} from '@s4wave/web/forge/predicates.js'
import { loadForgeTaskDependencyGraph } from './useForgeTaskDependencyGraph.js'

describe('loadForgeTaskDependencyGraph', () => {
  it('groups dependency lookups by predicate instead of querying per task', async () => {
    const signal = new AbortController().signal
    const world = {
      listGraphEdgeBuckets: vi.fn(
        (
          originObjectKeys: string[],
          _limitPerOrigin: number,
          options: { predicate?: string },
        ) =>
          Promise.resolve({
            buckets: originObjectKeys.map((originObjectKey) => ({
              originObjectKey,
              outgoing:
                originObjectKey === 'forge/task/1'
                  ? [
                      {
                        obj: keyToIRI(
                          options.predicate === PRED_TASK_TO_SUBTASK
                            ? 'forge/task/2'
                            : 'forge/task/3',
                        ),
                        predicate: options.predicate,
                      },
                      {
                        obj: keyToIRI('forge/task/outside'),
                        predicate: options.predicate,
                      },
                    ]
                  : [],
            })),
          }),
      ),
      lookupGraphQuads: vi.fn(() => {
        throw new Error('use listGraphEdgeBuckets')
      }),
    } as unknown as IWorldState

    const edges = await loadForgeTaskDependencyGraph(
      world,
      [
        { objectKey: 'forge/task/1', typeId: 'forge/task' },
        { objectKey: 'forge/task/2', typeId: 'forge/task' },
        { objectKey: 'forge/task/3', typeId: 'forge/task' },
      ],
      signal,
    )

    expect(edges).toEqual([
      { from: 'forge/task/1', to: 'forge/task/2', kind: 'subtask' },
      { from: 'forge/task/1', to: 'forge/task/3', kind: 'cached' },
    ])
    expect(world.lookupGraphQuads).not.toHaveBeenCalled()
    expect(world.listGraphEdgeBuckets).toHaveBeenCalledTimes(2)
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      1,
      ['forge/task/1', 'forge/task/2', 'forge/task/3'],
      50,
      {
        predicate: PRED_TASK_TO_SUBTASK,
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      2,
      ['forge/task/1', 'forge/task/2', 'forge/task/3'],
      50,
      {
        predicate: PRED_TASK_TO_CACHED,
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      },
    )
  })
})
