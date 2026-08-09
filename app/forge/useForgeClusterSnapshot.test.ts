import { describe, expect, it, vi } from 'vitest'

import {
  Execution,
  State as ExecutionState,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import {
  Job,
  State as JobState,
} from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import {
  Pass,
  State as PassState,
} from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import {
  Task,
  State as TaskState,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { Worker } from '@go/github.com/s4wave/spacewave/forge/worker/worker.pb.js'
import { Keypair } from '@go/github.com/s4wave/spacewave/identity/identity.pb.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import {
  PRED_CLUSTER_TO_JOB,
  PRED_CLUSTER_TO_WORKER,
  PRED_JOB_TO_TASK,
  PRED_OBJECT_TO_KEYPAIR,
  PRED_PASS_TO_EXECUTION,
  PRED_TASK_TO_PASS,
} from '@s4wave/web/forge/predicates.js'

import { buildForgeClusterSnapshot } from './useForgeClusterSnapshot.js'

interface TestEdge {
  predicate: string
  target: string
}

function disposable<T extends object>(value: T): T & Disposable {
  return Object.assign(value, {
    [Symbol.dispose]() {},
  })
}

describe('buildForgeClusterSnapshot', () => {
  it('uses grouped edge buckets for the first cluster snapshot', async () => {
    const signal = new AbortController().signal
    const objectData = new Map<string, Uint8Array>([
      [
        'forge/job/1',
        Job.toBinary({
          jobState: JobState.JobState_RUNNING,
          timestamp: new Date('2026-05-11T12:00:00Z'),
        }),
      ],
      [
        'forge/task/1',
        Task.toBinary({
          taskState: TaskState.TaskState_CHECKING,
          timestamp: new Date('2026-05-11T12:01:00Z'),
        }),
      ],
      [
        'forge/pass/1',
        Pass.toBinary({
          passState: PassState.PassState_RUNNING,
          passNonce: 1n,
          timestamp: new Date('2026-05-11T12:02:00Z'),
        }),
      ],
      [
        'forge/execution/1',
        Execution.toBinary({
          executionState: ExecutionState.ExecutionState_RUNNING,
          peerId: '12D3KooWworker',
          timestamp: new Date('2026-05-11T12:03:00Z'),
        }),
      ],
      ['forge/worker/1', Worker.toBinary({ name: 'worker-1' })],
      ['identity/keypair/1', Keypair.toBinary({ peerId: '12D3KooWworker' })],
    ])
    const edges = new Map<string, TestEdge[]>([
      [
        'forge/cluster/main',
        [
          { predicate: PRED_CLUSTER_TO_JOB, target: 'forge/job/1' },
          { predicate: PRED_CLUSTER_TO_WORKER, target: 'forge/worker/1' },
        ],
      ],
      [
        'forge/job/1',
        [{ predicate: PRED_JOB_TO_TASK, target: 'forge/task/1' }],
      ],
      [
        'forge/task/1',
        [{ predicate: PRED_TASK_TO_PASS, target: 'forge/pass/1' }],
      ],
      [
        'forge/pass/1',
        [{ predicate: PRED_PASS_TO_EXECUTION, target: 'forge/execution/1' }],
      ],
      [
        'forge/worker/1',
        [{ predicate: PRED_OBJECT_TO_KEYPAIR, target: 'identity/keypair/1' }],
      ],
    ])

    const world = {
      listGraphEdgeBuckets: vi.fn(
        (
          originObjectKeys: string[],
          limitPerOrigin: number,
          options: { direction?: GraphEdgeBucketDirection },
        ) => {
          expect(limitPerOrigin).toBe(200)
          expect(options.direction).toBe(GraphEdgeBucketDirection.OUT)
          return Promise.resolve({
            buckets: originObjectKeys.map((originObjectKey) => ({
              originObjectKey,
              outgoing: (edges.get(originObjectKey) ?? []).map((edge) => ({
                subject: keyToIRI(originObjectKey),
                predicate: edge.predicate,
                obj: keyToIRI(edge.target),
              })),
            })),
          })
        },
      ),
      lookupGraphQuads: vi.fn(() => {
        throw new Error('use listGraphEdgeBuckets for snapshot edge lookup')
      }),
      getObject: vi.fn((objectKey: string) => {
        const data = objectData.get(objectKey)
        if (!data) return Promise.resolve(null)
        return Promise.resolve(
          disposable({
            accessWorldState: vi.fn(() =>
              Promise.resolve(
                disposable({
                  unmarshal: vi.fn(() =>
                    Promise.resolve({ found: true, data }),
                  ),
                }),
              ),
            ),
          }),
        )
      }),
    } as unknown as IWorldState

    const snapshot = await buildForgeClusterSnapshot(
      world,
      ['forge/cluster/main'],
      signal,
    )

    expect(snapshot.jobs.map((job) => job.objectKey)).toEqual(['forge/job/1'])
    expect(snapshot.tasks.map((task) => task.objectKey)).toEqual([
      'forge/task/1',
    ])
    expect(snapshot.passes.map((pass) => pass.objectKey)).toEqual([
      'forge/pass/1',
    ])
    expect(snapshot.executions.map((execution) => execution.objectKey)).toEqual(
      ['forge/execution/1'],
    )
    expect(snapshot.workers).toMatchObject([
      {
        objectKey: 'forge/worker/1',
        clusterKeys: ['forge/cluster/main'],
        keypairKeys: ['identity/keypair/1'],
        peerIds: ['12D3KooWworker'],
      },
    ])
    expect(world.lookupGraphQuads).not.toHaveBeenCalled()
    expect(world.listGraphEdgeBuckets).toHaveBeenCalledTimes(5)
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      1,
      ['forge/cluster/main'],
      200,
      { direction: GraphEdgeBucketDirection.OUT, abortSignal: signal },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      2,
      ['forge/job/1'],
      200,
      { direction: GraphEdgeBucketDirection.OUT, abortSignal: signal },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      3,
      ['forge/worker/1'],
      200,
      { direction: GraphEdgeBucketDirection.OUT, abortSignal: signal },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      4,
      ['forge/task/1'],
      200,
      { direction: GraphEdgeBucketDirection.OUT, abortSignal: signal },
    )
    expect(world.listGraphEdgeBuckets).toHaveBeenNthCalledWith(
      5,
      ['forge/pass/1'],
      200,
      { direction: GraphEdgeBucketDirection.OUT, abortSignal: signal },
    )
  })

  it('rejects a truncated graph snapshot', async () => {
    const world = {
      listGraphEdgeBuckets: vi.fn(() =>
        Promise.resolve({
          buckets: [
            {
              originObjectKey: 'forge/cluster/main',
              outgoing: [],
              outgoingTruncated: true,
            },
          ],
        }),
      ),
    } as unknown as IWorldState

    await expect(
      buildForgeClusterSnapshot(
        world,
        ['forge/cluster/main'],
        new AbortController().signal,
      ),
    ).rejects.toThrow('exceeds the 200-edge limit')
  })
})
