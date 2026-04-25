import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
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

export function useForgeTaskDependencyGraph(
  worldState: Resource<IWorldState>,
  tasks: ForgeLinkedEntity[],
): { edges: ForgeTaskDependencyEdge[]; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world) return []

      const taskKeys = new Set(tasks.map((task) => task.objectKey))
      const edgeSets = await Promise.all(
        tasks.map(async (task) => {
          const queryEdgeSet = async (
            predicate: string,
            kind: ForgeTaskDependencyEdge['kind'],
          ) => {
            const result = await world.lookupGraphQuads(
              keyToIRI(task.objectKey),
              predicate,
              undefined,
              undefined,
              50,
              signal,
            )
            return (result.quads ?? [])
              .map((quad) => quad.obj)
              .filter((obj): obj is string => !!obj)
              .map((obj) => iriToKey(obj))
              .filter((objKey) => taskKeys.has(objKey))
              .map((objKey) => ({
                from: task.objectKey,
                to: objKey,
                kind,
              }))
          }

          const [subtaskEdges, cachedEdges] = await Promise.all([
            queryEdgeSet(PRED_TASK_TO_SUBTASK, 'subtask'),
            queryEdgeSet(PRED_TASK_TO_CACHED, 'cached'),
          ])

          return [...subtaskEdges, ...cachedEdges]
        }),
      )

      return edgeSets.flat()
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
