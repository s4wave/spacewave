import { describe, expect, it, vi } from 'vitest'

import {
  readKeyedSet,
  type KeyedSetReader,
} from '@s4wave/sdk/world/entity-set.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import {
  PRED_JOB_TO_TASK,
  PRED_TASK_TO_CACHED,
  PRED_TASK_TO_SUBTASK,
} from '@s4wave/web/forge/predicates.js'
import {
  forgeTaskDependencyEdges,
  forgeTaskDependencySource,
} from './useForgeTaskDependencyGraph.js'

const tasks = ['forge/task/1', 'forge/task/2', 'forge/task/3']

describe('forgeTaskDependencySource', () => {
  it('reads the edges among the job tasks in one call per predicate', async () => {
    const signal = new AbortController().signal
    const reader = {
      getSeqno: vi.fn(() => Promise.resolve({ seqno: 4n })),
      listGraphEdgeBuckets: vi.fn(
        (keys: string[], _limit: number, options: { predicate?: string }) =>
          Promise.resolve({
            buckets:
              options.predicate === PRED_JOB_TO_TASK
                ? [
                    {
                      originObjectKey: 'forge/job',
                      outgoing: tasks.map((key) => ({ obj: keyToIRI(key) })),
                    },
                  ]
                : keys.map((originObjectKey) => ({
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
                            },
                            { obj: keyToIRI('forge/task/outside') },
                          ]
                        : [],
                  })),
          }),
      ),
    }

    const set = await readKeyedSet(
      reader as unknown as KeyedSetReader,
      forgeTaskDependencySource('forge/job'),
      signal,
    )

    expect(forgeTaskDependencyEdges(set)).toEqual([
      { from: 'forge/task/1', to: 'forge/task/2', kind: 'subtask' },
      { from: 'forge/task/1', to: 'forge/task/3', kind: 'cached' },
    ])
    for (const predicate of [PRED_TASK_TO_SUBTASK, PRED_TASK_TO_CACHED]) {
      expect(reader.listGraphEdgeBuckets).toHaveBeenCalledWith(tasks, 50, {
        predicate,
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      })
    }
  })

  it('takes a task added to the job as a change of the set', () => {
    const source = forgeTaskDependencySource('forge/job')
    const members = new Set(tasks)
    const added = source.route(
      {
        seqno: 5n,
        quads: [
          {
            subject: keyToIRI('forge/job'),
            predicate: PRED_JOB_TO_TASK,
            obj: keyToIRI('forge/task/4'),
          },
        ],
      },
      members,
    )
    expect(added).toEqual({ list: true, keys: [] })

    const linked = source.route(
      {
        seqno: 6n,
        quads: [
          {
            subject: keyToIRI('forge/task/4'),
            predicate: PRED_TASK_TO_SUBTASK,
            obj: keyToIRI('forge/task/1'),
          },
        ],
      },
      members,
    )
    expect(linked).toEqual({ list: false, keys: ['forge/task/4'] })

    const blank = source.route(
      {
        seqno: 7n,
        quads: [
          {
            subject: '_:b0',
            predicate: PRED_TASK_TO_SUBTASK,
            obj: keyToIRI('forge/task/1'),
          },
        ],
      },
      members,
    )
    expect(blank).toEqual({ list: false, keys: [] })
    expect(source.names(['forge/task/4']).patterns).toHaveLength(2)
  })
})
