import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import type { ForgeLinkedEntity } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import {
  PRED_TASK_TO_CACHED,
  PRED_TASK_TO_SUBTASK,
} from '@s4wave/web/forge/predicates.js'

export interface ForgeTaskDependencyEdge {
  from: string
  to: string
  kind: 'subtask' | 'cached'
}

const forgeTaskDependencyLimit = 50

type EdgeBucket = NonNullable<
  Awaited<ReturnType<IWorldState['listGraphEdgeBuckets']>>['buckets']
>[number]

function dependencyEdgesFromBucket(
  bucket: EdgeBucket | undefined,
  taskKeys: Set<string>,
  kind: ForgeTaskDependencyEdge['kind'],
): ForgeTaskDependencyEdge[] {
  const from = bucket?.originObjectKey ?? ''
  if (!from) return []
  return (bucket?.outgoing ?? []).flatMap((quad) => {
    if (!quad.obj) return []
    const to = iriToKey(quad.obj)
    return taskKeys.has(to) ? [{ from, to, kind }] : []
  })
}

async function loadDependencyBuckets(
  world: IWorldState,
  taskKeys: string[],
  predicate: string,
  signal: AbortSignal,
): Promise<Map<string, EdgeBucket>> {
  if (taskKeys.length === 0) return new Map()
  const response = await world.listGraphEdgeBuckets(
    taskKeys,
    forgeTaskDependencyLimit,
    {
      predicate,
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

export async function loadForgeTaskDependencyGraph(
  world: IWorldState,
  tasks: ForgeLinkedEntity[],
  signal: AbortSignal,
): Promise<ForgeTaskDependencyEdge[]> {
  const taskKeys = [...new Set(tasks.map((task) => task.objectKey))]
  const taskKeySet = new Set(taskKeys)
  const [subtaskBuckets, cachedBuckets] = await Promise.all([
    loadDependencyBuckets(world, taskKeys, PRED_TASK_TO_SUBTASK, signal),
    loadDependencyBuckets(world, taskKeys, PRED_TASK_TO_CACHED, signal),
  ])

  return taskKeys.flatMap((taskKey) => [
    ...dependencyEdgesFromBucket(
      subtaskBuckets.get(taskKey),
      taskKeySet,
      'subtask',
    ),
    ...dependencyEdgesFromBucket(
      cachedBuckets.get(taskKey),
      taskKeySet,
      'cached',
    ),
  ])
}

export function useForgeTaskDependencyGraph(
  worldState: Resource<IWorldState>,
  tasks: ForgeLinkedEntity[],
): { edges: ForgeTaskDependencyEdge[]; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world) return []
      return loadForgeTaskDependencyGraph(world, tasks, signal)
    },
    [tasks],
  )

  return useMemo(
    () => ({
      edges: resource.value ?? [],
      loading: resource.loading,
    }),
    [resource.loading, resource.value],
  )
}
