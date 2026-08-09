import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import {
  PRED_TASK_TO_CACHED,
  PRED_TASK_TO_SUBTASK,
} from '@s4wave/web/forge/predicates.js'

import {
  loadForgeTaskDependencyGraph,
  useForgeTaskDependencyGraph,
} from './useForgeTaskDependencyGraph.js'

afterEach(cleanup)

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

  it('rejects a truncated dependency graph', async () => {
    const world = {
      listGraphEdgeBuckets: vi.fn(() =>
        Promise.resolve({
          buckets: [
            {
              originObjectKey: 'forge/task/1',
              outgoing: [],
              outgoingTruncated: true,
            },
          ],
        }),
      ),
    } as unknown as IWorldState

    await expect(
      loadForgeTaskDependencyGraph(
        world,
        [{ objectKey: 'forge/task/1', typeId: 'forge/task' }],
        new AbortController().signal,
      ),
    ).rejects.toThrow('exceeds the 50-edge limit')
  })
})

describe('useForgeTaskDependencyGraph', () => {
  const parentResource = (world: IWorldState): Resource<IWorldState> => ({
    value: world,
    loading: false,
    error: null,
    retry: vi.fn(),
  })

  it('retains the last published graph across a failed refresh and replaces it on recovery', async () => {
    const world = {
      listGraphEdgeBuckets: vi.fn(
        (
          taskKeys: string[],
          _limitPerOrigin: number,
          options: { predicate?: string },
        ) => {
          if (taskKeys.includes('forge/task/truncated')) {
            return Promise.resolve({
              buckets: [
                {
                  originObjectKey: 'forge/task/1',
                  outgoing: [],
                  outgoingTruncated: true,
                },
              ],
            })
          }
          return Promise.resolve({
            buckets: taskKeys.map((originObjectKey) => ({
              originObjectKey,
              outgoing:
                originObjectKey === 'forge/task/1' &&
                options.predicate === PRED_TASK_TO_SUBTASK
                  ? [
                      {
                        predicate: options.predicate,
                        obj: keyToIRI(taskKeys[1]!),
                      },
                    ]
                  : [],
            })),
          })
        },
      ),
    } as unknown as IWorldState
    const tasks = (target: string) => [
      { objectKey: 'forge/task/1', typeId: 'forge/task' },
      { objectKey: target, typeId: 'forge/task' },
    ]
    const worldResource = parentResource(world)
    const { result, rerender } = renderHook(
      ({ taskList }) => useForgeTaskDependencyGraph(worldResource, taskList),
      { initialProps: { taskList: tasks('forge/task/2') } },
    )

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current).toMatchObject({
      edges: [
        {
          from: 'forge/task/1',
          to: 'forge/task/2',
          kind: 'subtask',
        },
      ],
      error: null,
    })

    rerender({ taskList: tasks('forge/task/truncated') })
    await waitFor(() =>
      expect(result.current.error?.message).toContain(
        'exceeds the 50-edge limit',
      ),
    )
    expect(result.current.edges).toEqual([
      { from: 'forge/task/1', to: 'forge/task/2', kind: 'subtask' },
    ])

    rerender({ taskList: tasks('forge/task/3') })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current).toMatchObject({
      edges: [
        {
          from: 'forge/task/1',
          to: 'forge/task/3',
          kind: 'subtask',
        },
      ],
      error: null,
    })
  })

  it('publishes an empty graph when the initial load fails', async () => {
    const world = {
      listGraphEdgeBuckets: vi.fn(() =>
        Promise.resolve({
          buckets: [
            {
              originObjectKey: 'forge/task/1',
              outgoing: [],
              outgoingTruncated: true,
            },
          ],
        }),
      ),
    } as unknown as IWorldState
    const worldResource = parentResource(world)
    const tasks = [{ objectKey: 'forge/task/1', typeId: 'forge/task' }]
    const { result } = renderHook(() =>
      useForgeTaskDependencyGraph(worldResource, tasks),
    )

    await waitFor(() => expect(result.current.error).not.toBeNull())
    expect(result.current.edges).toEqual([])
  })
})
