import { useMemo } from 'react'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'

// ForgeLinkedEntity represents a linked entity discovered via graph quads.
export interface ForgeLinkedEntity {
  objectKey: string
  typeId: string
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
      const iri = keyToIRI(objectKey)

      let result
      if (direction === 'out') {
        result = await world.lookupGraphQuads(
          iri,
          predicate,
          undefined,
          undefined,
          200,
          signal,
        )
      } else {
        result = await world.lookupGraphQuads(
          undefined,
          predicate,
          iri,
          undefined,
          200,
          signal,
        )
      }

      const quads = result.quads ?? []
      const items = quads
        .map((q) => (direction === 'out' ? q.obj : q.subject))
        .filter((iri): iri is string => !!iri)
      const entities = await Promise.all(
        items.map(async (entityIRI) => {
          const key = iriToKey(entityIRI)
          const typeResult = await world.lookupGraphQuads(
            keyToIRI(key),
            '<type>',
            undefined,
            undefined,
            1,
            signal,
          )
          const typeId =
            typeResult.quads?.[0]?.obj ? iriToKey(typeResult.quads[0].obj) : ''
          return { objectKey: key, typeId }
        }),
      )
      return entities
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
