import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Execution } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import { Job } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { Pass } from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import { Task } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { Worker } from '@go/github.com/s4wave/spacewave/forge/worker/worker.pb.js'
import { Keypair } from '@go/github.com/s4wave/spacewave/identity/identity.pb.js'
import { iriToKey } from '@s4wave/sdk/world/graph-utils.js'
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

interface ForgeClusterSnapshotNode<T> {
  objectKey: string
  data: T
}

export interface ForgeClusterJobSnapshot extends ForgeClusterSnapshotNode<Job> {
  clusterKey: string
  taskKeys: string[]
}

export interface ForgeClusterTaskSnapshot extends ForgeClusterSnapshotNode<Task> {
  clusterKey: string
  jobKey: string
  passKeys: string[]
}

export interface ForgeClusterPassSnapshot extends ForgeClusterSnapshotNode<Pass> {
  clusterKey: string
  jobKey: string
  taskKey: string
  executionKeys: string[]
}

export interface ForgeClusterExecutionSnapshot extends ForgeClusterSnapshotNode<Execution> {
  clusterKey: string
  jobKey: string
  taskKey: string
  passKey: string
}

export interface ForgeClusterWorkerSnapshot extends ForgeClusterSnapshotNode<Worker> {
  clusterKeys: string[]
  keypairKeys: string[]
  peerIds: string[]
}

export interface ForgeClusterSnapshot {
  jobs: ForgeClusterJobSnapshot[]
  tasks: ForgeClusterTaskSnapshot[]
  passes: ForgeClusterPassSnapshot[]
  executions: ForgeClusterExecutionSnapshot[]
  workers: ForgeClusterWorkerSnapshot[]
}

const forgeEdgeLookupLimit = 200

function emptyForgeClusterSnapshot(): ForgeClusterSnapshot {
  return {
    jobs: [],
    tasks: [],
    passes: [],
    executions: [],
    workers: [],
  }
}

type EdgeBucket = NonNullable<
  Awaited<ReturnType<IWorldState['listGraphEdgeBuckets']>>['buckets']
>[number]

async function listOutgoingEdgeBuckets(
  world: IWorldState,
  objectKeys: string[],
  signal: AbortSignal,
): Promise<Map<string, EdgeBucket>> {
  const uniqueKeys = [...new Set(objectKeys)].filter(Boolean)
  if (uniqueKeys.length === 0) return new Map()

  const response = await world.listGraphEdgeBuckets(
    uniqueKeys,
    forgeEdgeLookupLimit,
    {
      direction: GraphEdgeBucketDirection.OUT,
      abortSignal: signal,
    },
  )
  return new Map(
    (response.buckets ?? []).map((bucket) => [
      bucket.originObjectKey ?? '',
      bucket,
    ]),
  )
}

function outgoingLinkedKeys(
  buckets: Map<string, EdgeBucket>,
  objectKey: string,
  predicate: string,
): string[] {
  const bucket = buckets.get(objectKey)
  return (bucket?.outgoing ?? []).flatMap((quad) =>
    quad.predicate === predicate && quad.obj ? [iriToKey(quad.obj)] : [],
  )
}

async function decodeObject<T extends { fromBinary(data: Uint8Array): U }, U>(
  world: IWorldState,
  objectKey: string,
  messageType: T,
  signal: AbortSignal,
): Promise<U | null> {
  using objectState = await world.getObject(objectKey, signal)
  if (!objectState) return null
  using cursor = await objectState.accessWorldState(undefined, signal)
  const resp = await cursor.unmarshal({}, signal)
  if (!resp.found || !resp.data?.length) return null
  return messageType.fromBinary(resp.data)
}

async function decodeObjectMap<
  T extends { fromBinary(data: Uint8Array): U },
  U,
>(
  world: IWorldState,
  objectKeys: string[],
  messageType: T,
  signal: AbortSignal,
): Promise<Map<string, U>> {
  const uniqueKeys = [...new Set(objectKeys)].filter(Boolean)
  const entries = await Promise.all(
    uniqueKeys.map(async (objectKey) => {
      const data = await decodeObject<T, U>(
        world,
        objectKey,
        messageType,
        signal,
      )
      const entry: readonly [string, U | null] = [objectKey, data]
      return entry
    }),
  )
  return new Map(
    entries.filter((entry): entry is readonly [string, U] => entry[1] !== null),
  )
}

export async function buildForgeClusterSnapshot(
  world: IWorldState,
  clusterKeys: string[],
  signal: AbortSignal,
): Promise<ForgeClusterSnapshot> {
  const uniqueClusterKeys = [...new Set(clusterKeys)].filter(Boolean)
  if (uniqueClusterKeys.length === 0) return emptyForgeClusterSnapshot()

  const clusterBuckets = await listOutgoingEdgeBuckets(
    world,
    uniqueClusterKeys,
    signal,
  )
  const jobKeysByCluster = new Map<string, string[]>()
  const workerKeysByCluster = new Map<string, string[]>()
  for (const clusterKey of uniqueClusterKeys) {
    jobKeysByCluster.set(
      clusterKey,
      outgoingLinkedKeys(clusterBuckets, clusterKey, PRED_CLUSTER_TO_JOB),
    )
    workerKeysByCluster.set(
      clusterKey,
      outgoingLinkedKeys(clusterBuckets, clusterKey, PRED_CLUSTER_TO_WORKER),
    )
  }

  const allJobKeys = [...jobKeysByCluster.values()].flat()
  const allWorkerKeys = [...workerKeysByCluster.values()].flat()
  const [jobData, workerData, jobBuckets, workerBuckets] = await Promise.all([
    decodeObjectMap(world, allJobKeys, Job, signal),
    decodeObjectMap(world, allWorkerKeys, Worker, signal),
    listOutgoingEdgeBuckets(world, allJobKeys, signal),
    listOutgoingEdgeBuckets(world, allWorkerKeys, signal),
  ])

  const taskKeysByJob = new Map<string, string[]>()
  for (const jobKey of new Set(allJobKeys)) {
    taskKeysByJob.set(
      jobKey,
      outgoingLinkedKeys(jobBuckets, jobKey, PRED_JOB_TO_TASK),
    )
  }
  const allTaskKeys = [...taskKeysByJob.values()].flat()
  const [taskData, taskBuckets] = await Promise.all([
    decodeObjectMap(world, allTaskKeys, Task, signal),
    listOutgoingEdgeBuckets(world, allTaskKeys, signal),
  ])

  const passKeysByTask = new Map<string, string[]>()
  for (const taskKey of new Set(allTaskKeys)) {
    passKeysByTask.set(
      taskKey,
      outgoingLinkedKeys(taskBuckets, taskKey, PRED_TASK_TO_PASS),
    )
  }
  const allPassKeys = [...passKeysByTask.values()].flat()
  const [passData, passBuckets] = await Promise.all([
    decodeObjectMap(world, allPassKeys, Pass, signal),
    listOutgoingEdgeBuckets(world, allPassKeys, signal),
  ])

  const executionKeysByPass = new Map<string, string[]>()
  for (const passKey of new Set(allPassKeys)) {
    executionKeysByPass.set(
      passKey,
      outgoingLinkedKeys(passBuckets, passKey, PRED_PASS_TO_EXECUTION),
    )
  }
  const allExecutionKeys = [...executionKeysByPass.values()].flat()
  const executionData = await decodeObjectMap(
    world,
    allExecutionKeys,
    Execution,
    signal,
  )

  const keypairKeysByWorker = new Map<string, string[]>()
  for (const workerKey of new Set(allWorkerKeys)) {
    keypairKeysByWorker.set(
      workerKey,
      outgoingLinkedKeys(workerBuckets, workerKey, PRED_OBJECT_TO_KEYPAIR),
    )
  }
  const keypairData = await decodeObjectMap<typeof Keypair, Keypair>(
    world,
    [...keypairKeysByWorker.values()].flat(),
    Keypair,
    signal,
  )

  const jobMap = new Map<string, ForgeClusterJobSnapshot>()
  const taskMap = new Map<string, ForgeClusterTaskSnapshot>()
  const passMap = new Map<string, ForgeClusterPassSnapshot>()
  const executionMap = new Map<string, ForgeClusterExecutionSnapshot>()
  const workerMap = new Map<string, ForgeClusterWorkerSnapshot>()

  for (const clusterKey of uniqueClusterKeys) {
    for (const jobKey of jobKeysByCluster.get(clusterKey) ?? []) {
      const job = jobData.get(jobKey)
      if (!job) continue

      const taskKeys = taskKeysByJob.get(jobKey) ?? []
      jobMap.set(jobKey, {
        objectKey: jobKey,
        clusterKey,
        data: job,
        taskKeys,
      })

      for (const taskKey of taskKeys) {
        const task = taskData.get(taskKey)
        if (!task) continue

        const passKeys = passKeysByTask.get(taskKey) ?? []
        taskMap.set(taskKey, {
          objectKey: taskKey,
          clusterKey,
          jobKey,
          data: task,
          passKeys,
        })

        for (const passKey of passKeys) {
          const pass = passData.get(passKey)
          if (!pass) continue

          const executionKeys = executionKeysByPass.get(passKey) ?? []
          passMap.set(passKey, {
            objectKey: passKey,
            clusterKey,
            jobKey,
            taskKey,
            data: pass,
            executionKeys,
          })

          for (const executionKey of executionKeys) {
            const execution = executionData.get(executionKey)
            if (!execution) continue

            executionMap.set(executionKey, {
              objectKey: executionKey,
              clusterKey,
              jobKey,
              taskKey,
              passKey,
              data: execution,
            })
          }
        }
      }
    }

    for (const workerKey of workerKeysByCluster.get(clusterKey) ?? []) {
      const worker = workerData.get(workerKey)
      if (!worker) continue

      const keypairKeys = keypairKeysByWorker.get(workerKey) ?? []
      const prev = workerMap.get(workerKey)
      workerMap.set(workerKey, {
        objectKey: workerKey,
        data: worker,
        clusterKeys: [...new Set([...(prev?.clusterKeys ?? []), clusterKey])],
        keypairKeys,
        peerIds: keypairKeys.flatMap((keypairKey) => {
          const keypair = keypairData.get(keypairKey)
          return keypair?.peerId ? [keypair.peerId] : []
        }),
      })
    }
  }

  return {
    jobs: [...jobMap.values()],
    tasks: [...taskMap.values()],
    passes: [...passMap.values()],
    executions: [...executionMap.values()],
    workers: [...workerMap.values()],
  }
}

export function useForgeClusterSnapshot(
  worldState: Resource<IWorldState>,
  clusterKeys: string[],
): { snapshot: ForgeClusterSnapshot; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world || clusterKeys.length === 0) {
        return emptyForgeClusterSnapshot()
      }

      return buildForgeClusterSnapshot(world, clusterKeys, signal)
    },
    [clusterKeys],
  )

  return useMemo(
    () => ({
      snapshot: resource.value ?? emptyForgeClusterSnapshot(),
      loading: resource.loading,
    }),
    [resource.loading, resource.value],
  )
}
