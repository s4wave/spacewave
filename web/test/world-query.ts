import { useEffect, useState, type DependencyList } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { WorldQuery } from '@s4wave/sdk/world/world-query.js'

// TypedObject is a seeded World object for a fake World.
export interface TypedObject {
  objectKey: string
  objectType: string
}

// typedObjectsWorld answers type queries from a fixed object list. Getting
// objects is a function so a test can reseed between renders.
export function typedObjectsWorld(getObjects: () => TypedObject[]) {
  return {
    listObjectsWithType: (typeID: string) =>
      Promise.resolve(
        getObjects()
          .filter((obj) => obj.objectType === typeID)
          .map((obj) => obj.objectKey),
      ),
  } as Partial<IWorldState> as IWorldState
}

// keysWorld answers listing pages from a key list. Getting keys may be a
// function so a test can reseed between renders. Its small pages exercise
// cursors.
export function keysWorld(
  keys: string[] | (() => string[]),
  pageSize = 2,
): IWorldState {
  return {
    listObjects: (prefix: string, startAfter: string, limit: number) => {
      const sorted = [...(typeof keys === 'function' ? keys() : keys)].sort()
      const under = sorted.filter(
        (key) => key.startsWith(prefix) && key > startAfter,
      )
      const page = under.slice(0, Math.min(limit, pageSize))
      return Promise.resolve({
        objects: page.map((objectKey) => ({ objectKey })),
        more: under.length > page.length,
      })
    },
  } as Partial<IWorldState> as IWorldState
}

// useFakeWorldQuery stands in for useWorldQuery: it runs the query against
// world when deps change and settles to its answer.
export function useFakeWorldQuery<T>(
  world: IWorldState,
  query: WorldQuery<T>,
  deps: DependencyList,
): Resource<T> {
  const [value, setValue] = useState<T | null>(null)
  useEffect(() => {
    void query(world).then(setValue)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [world, ...deps])
  return { value, loading: value === null, error: null, retry: () => {} }
}
