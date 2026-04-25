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
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
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

async function lookupLinkedKeys(
  world: IWorldState,
  objectKey: string,
  predicate: string,
  signal: AbortSignal,
): Promise<string[]> {
  const result = await world.lookupGraphQuads(
    keyToIRI(objectKey),
    predicate,
    undefined,
    undefined,
    200,
    signal,
  )
  return (result.quads ?? [])
    .map((quad) => quad.obj)
    .filter((obj): obj is string => !!obj)
    .map((obj) => iriToKey(obj))
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

export function useForgeClusterSnapshot(
  worldState: Resource<IWorldState>,
  clusterKeys: string[],
): { snapshot: ForgeClusterSnapshot; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world || clusterKeys.length === 0) {
        return {
          jobs: [],
          tasks: [],
          passes: [],
          executions: [],
          workers: [],
        } satisfies ForgeClusterSnapshot
      }

      const uniqueClusterKeys = [...new Set(clusterKeys)].filter(Boolean)
      const jobMap = new Map<string, ForgeClusterJobSnapshot>()
      const taskMap = new Map<string, ForgeClusterTaskSnapshot>()
      const passMap = new Map<string, ForgeClusterPassSnapshot>()
      const executionMap = new Map<string, ForgeClusterExecutionSnapshot>()
      const workerMap = new Map<string, ForgeClusterWorkerSnapshot>()

      for (const clusterKey of uniqueClusterKeys) {
        const [jobKeys, workerKeys] = await Promise.all([
          lookupLinkedKeys(world, clusterKey, PRED_CLUSTER_TO_JOB, signal),
          lookupLinkedKeys(world, clusterKey, PRED_CLUSTER_TO_WORKER, signal),
        ])

        for (const jobKey of jobKeys) {
          const job = await decodeObject(world, jobKey, Job, signal)
          if (!job) continue

          const taskKeys = await lookupLinkedKeys(
            world,
            jobKey,
            PRED_JOB_TO_TASK,
            signal,
          )
          jobMap.set(jobKey, {
            objectKey: jobKey,
            clusterKey,
            data: job,
            taskKeys,
          })

          for (const taskKey of taskKeys) {
            const task = await decodeObject(world, taskKey, Task, signal)
            if (!task) continue

            const passKeys = await lookupLinkedKeys(
              world,
              taskKey,
              PRED_TASK_TO_PASS,
              signal,
            )
            taskMap.set(taskKey, {
              objectKey: taskKey,
              clusterKey,
              jobKey,
              data: task,
              passKeys,
            })

            for (const passKey of passKeys) {
              const pass = await decodeObject(world, passKey, Pass, signal)
              if (!pass) continue

              const executionKeys = await lookupLinkedKeys(
                world,
                passKey,
                PRED_PASS_TO_EXECUTION,
                signal,
              )
              passMap.set(passKey, {
                objectKey: passKey,
                clusterKey,
                jobKey,
                taskKey,
                data: pass,
                executionKeys,
              })

              for (const executionKey of executionKeys) {
                const execution = await decodeObject(
                  world,
                  executionKey,
                  Execution,
                  signal,
                )
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

        for (const workerKey of workerKeys) {
          const worker = await decodeObject(world, workerKey, Worker, signal)
          if (!worker) continue

          const keypairKeys = await lookupLinkedKeys(
            world,
            workerKey,
            PRED_OBJECT_TO_KEYPAIR,
            signal,
          )
          const keypairs = await Promise.all(
            keypairKeys.map((keypairKey) =>
              decodeObject(world, keypairKey, Keypair, signal),
            ),
          )
          const prev = workerMap.get(workerKey)
          workerMap.set(workerKey, {
            objectKey: workerKey,
            data: worker,
            clusterKeys: [
              ...new Set([...(prev?.clusterKeys ?? []), clusterKey]),
            ],
            keypairKeys,
            peerIds: keypairs
              .filter((keypair): keypair is Keypair => keypair !== null)
              .map((keypair) => keypair.peerId ?? '')
              .filter(Boolean),
          })
        }
      }

      return {
        jobs: [...jobMap.values()],
        tasks: [...taskMap.values()],
        passes: [...passMap.values()],
        executions: [...executionMap.values()],
        workers: [...workerMap.values()],
      } satisfies ForgeClusterSnapshot
    },
    [clusterKeys],
  )

  return useMemo(
    () => ({
      snapshot:
        resource.value ??
        ({
          jobs: [],
          tasks: [],
          passes: [],
          executions: [],
          workers: [],
        } satisfies ForgeClusterSnapshot),
      loading: resource.loading,
    }),
    [resource.loading, resource.value],
  )
}
