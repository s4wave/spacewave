import { useMemo } from 'react'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'

// ForgeLinkedEntity represents a linked entity discovered via graph quads.
export interface ForgeLinkedEntity {
  objectKey: string
  typeId: string
}

const forgeLinkedEntityLimit = 200
const forgeLinkedEntityTypeLimit = 1
const typePredicate = '<type>'

export async function loadForgeLinkedEntities(
  world: IWorldState,
  objectKey: string,
  predicate: string,
  direction: 'out' | 'in',
  signal: AbortSignal,
): Promise<ForgeLinkedEntity[]> {
  if (!objectKey) return []

  const bucketResponse = await world.listGraphEdgeBuckets(
    [objectKey],
    forgeLinkedEntityLimit,
    {
      predicate,
      direction:
        direction === 'out'
          ? GraphEdgeBucketDirection.OUT
          : GraphEdgeBucketDirection.IN,
      abortSignal: signal,
    },
  )
  const bucket = bucketResponse.buckets?.[0]
  const linkedIRIs =
    direction === 'out'
      ? (bucket?.outgoing ?? []).flatMap((q) => (q.obj ? [q.obj] : []))
      : (bucket?.incoming ?? []).flatMap((q) => (q.subject ? [q.subject] : []))
  const entityKeys = linkedIRIs.map(iriToKey)
  if (entityKeys.length === 0) return []

  const uniqueEntityKeys = [...new Set(entityKeys)]
  const typeResponse = await world.listGraphEdgeBuckets(
    uniqueEntityKeys,
    forgeLinkedEntityTypeLimit,
    {
      predicate: typePredicate,
      direction: GraphEdgeBucketDirection.OUT,
      abortSignal: signal,
    },
  )
  const typeIdsByEntity = new Map(
    (typeResponse.buckets ?? []).map((typeBucket) => [
      typeBucket.originObjectKey ?? '',
      typeBucket.outgoing?.[0]?.obj ? iriToKey(typeBucket.outgoing[0].obj) : '',
    ]),
  )

  return entityKeys.map((key) => ({
    objectKey: key,
    typeId: typeIdsByEntity.get(key) ?? '',
  }))
}

// useForgeLinkedEntities queries graph quads to find entities linked to the
// given object key via the specified predicate. Returns entity keys with their
// types. Direction controls traversal: 'out' follows subject->object edges,
// 'in' follows object->subject edges.
export function useForgeLinkedEntities(
  worldState: Resource<IWorldState>,
  objectKey: string,
  predicate: string,
  direction: 'out' | 'in' = 'out',
): { entities: ForgeLinkedEntity[]; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      if (!world || !objectKey) return []
      return loadForgeLinkedEntities(
        world,
        objectKey,
        predicate,
        direction,
        signal,
      )
    },
    [objectKey, predicate, direction],
  )

  return useMemo(
    () => ({
      entities: resource.value ?? [],
      loading: resource.loading,
    }),
    [resource.value, resource.loading],
  )
}
