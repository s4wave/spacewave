import { useMemo } from 'react'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { changePattern } from '@s4wave/sdk/world/change-feed.js'
import {
  entitySetSource,
  readKeyedSet,
  watchKeyedSet,
  type KeyedSet,
  type KeyedSetReader,
  type KeyedSetSource,
} from '@s4wave/sdk/world/entity-set.js'
import { iriToKey, isIRI, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import {
  WorldStateResource,
  type IWorldState,
} from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import {
  PRED_JOB_TO_TASK,
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
  Awaited<ReturnType<KeyedSetReader['listGraphEdgeBuckets']>>['buckets']
>[number]

function dependencyEdgesFromBucket(
  bucket: EdgeBucket | undefined,
  kind: ForgeTaskDependencyEdge['kind'],
): ForgeTaskDependencyEdge[] {
  const from = bucket?.originObjectKey ?? ''
  if (!from) return []
  return (bucket?.outgoing ?? []).flatMap((quad) =>
    quad.obj ? [{ from, to: iriToKey(quad.obj), kind }] : [],
  )
}

async function loadDependencyBuckets(
  tx: Pick<KeyedSetReader, 'listGraphEdgeBuckets'>,
  keys: readonly string[],
  predicate: string,
  signal: AbortSignal,
): Promise<Map<string, EdgeBucket>> {
  const response = await tx.listGraphEdgeBuckets(
    [...keys],
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

// dependencyPredicates are the links a dependency edge follows.
const dependencyPredicates = new Set([
  PRED_TASK_TO_SUBTASK,
  PRED_TASK_TO_CACHED,
])

/**
 * forgeTaskDependencySource is the keyed set of a job's tasks, each with the
 * subtask and cached edges out of it. It lists the tasks by the job's task
 * links, so a task added to or removed from the job is a change of the set,
 * followed and read alone, and it wakes on a subtask or cached link only when
 * its subject is one of the tasks. A row holds every edge out of its task;
 * forgeTaskDependencyEdges keeps those between members.
 */
export function forgeTaskDependencySource(
  jobKey: string,
): KeyedSetSource<ForgeTaskDependencyEdge[]> {
  const tasks = entitySetSource(
    {
      case: 'link',
      origin: jobKey,
      predicate: PRED_JOB_TO_TASK,
      direction: 'out',
    },
    { name: 'none', decode: () => null },
  )
  return {
    members: tasks.members,
    list: tasks.list,
    names: (keys) => ({
      patterns: keys.flatMap((key) =>
        [...dependencyPredicates].map((predicate) =>
          changePattern({ subject: keyToIRI(key), predicate }),
        ),
      ),
    }),
    route: (change, members) => ({
      list: tasks.route(change, members).list,
      keys: (change.quads ?? []).flatMap((quad) =>
        dependencyPredicates.has(quad.predicate ?? '') && isIRI(quad.subject)
          ? [iriToKey(quad.subject)]
          : [],
      ),
    }),
    read: async function* (tx, keys, signal) {
      if (keys.length === 0) return
      const [subtasks, cached] = await Promise.all([
        loadDependencyBuckets(tx, keys, PRED_TASK_TO_SUBTASK, signal),
        loadDependencyBuckets(tx, keys, PRED_TASK_TO_CACHED, signal),
      ])
      yield {
        rows: keys.map(
          (key) =>
            [
              key,
              [
                ...dependencyEdgesFromBucket(subtasks.get(key), 'subtask'),
                ...dependencyEdgesFromBucket(cached.get(key), 'cached'),
              ],
            ] as const,
        ),
      }
    },
  }
}

// forgeTaskDependencyEdges returns the edges of set whose ends are both
// members, in member order.
export function forgeTaskDependencyEdges(
  set: KeyedSet<ForgeTaskDependencyEdge[]>,
): ForgeTaskDependencyEdge[] {
  const members = new Set(set.keys)
  return set.keys.flatMap((key) =>
    (set.rows.get(key) ?? []).filter((edge) => members.has(edge.to)),
  )
}

// useForgeTaskDependencyGraph follows the dependency edges among the tasks of
// the job at jobKey. A task added to or removed from the job reads only that
// task's edges, and a subtask or cached link out of a task reads only its
// edges again.
export function useForgeTaskDependencyGraph(
  worldState: Resource<IWorldState>,
  jobKey: string,
): { edges: ForgeTaskDependencyEdge[]; loading: boolean } {
  const resource = useStreamingResource(
    worldState,
    async function* (world, signal) {
      const source = forgeTaskDependencySource(jobKey)
      const engine = world.getEngine?.()
      if (engine) {
        for await (const set of watchKeyedSet(engine, source, signal)) {
          yield forgeTaskDependencyEdges(set)
        }
        return
      }
      if (!(world instanceof WorldStateResource)) {
        throw new Error('This World can not list its objects.')
      }
      yield forgeTaskDependencyEdges(await readKeyedSet(world, source, signal))
    },
    [jobKey],
    { reconnect: true },
  )

  return useMemo(
    () => ({
      edges: resourceContent(resource) ?? [],
      loading: resource.loading,
    }),
    [resource],
  )
}
