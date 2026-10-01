import { useEffect, useState, type DependencyList } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { ListObjectsRequest } from '@s4wave/sdk/world/world.pb.js'
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

// listingWorld answers listing pages from an object list, grouping keys when
// a request has a delimiter. Getting objects may be a function so a test can
// reseed between renders. Its small pages exercise cursors.
export function listingWorld(
  objects: TypedObject[] | (() => TypedObject[]),
  pageSize = 2,
): IWorldState {
  return {
    listObjects: ({
      prefix = '',
      delimiter = '',
      startAfter = '',
      limit = 0,
    }: ListObjectsRequest) => {
      const listed = typeof objects === 'function' ? objects() : objects
      const sorted = listed.toSorted((a, b) =>
        a.objectKey < b.objectKey ? -1 : 1,
      )

      // Collapse each key below the next delimiter into its group.
      const entries: { key: string; object?: TypedObject }[] = []
      for (const object of sorted) {
        if (!object.objectKey.startsWith(prefix)) continue
        const rest = object.objectKey.slice(prefix.length)
        const i = delimiter ? rest.indexOf(delimiter) : -1
        const key =
          i < 0
            ? object.objectKey
            : prefix + rest.slice(0, i + delimiter.length)
        if (key <= startAfter || entries.at(-1)?.key === key) continue
        entries.push(i < 0 ? { key, object } : { key })
      }

      const page = entries.slice(0, Math.min(limit, pageSize))
      return Promise.resolve({
        objects: page.flatMap(({ object }) =>
          object
            ? [{ objectKey: object.objectKey, typeId: object.objectType }]
            : [],
        ),
        prefixes: page.flatMap(({ key, object }) => (object ? [] : [key])),
        more: entries.length > page.length,
      })
    },
  } as Partial<IWorldState> as IWorldState
}

// keysWorld answers listing pages from a key list with no types.
export function keysWorld(
  keys: string[] | (() => string[]),
  pageSize = 2,
): IWorldState {
  const typed = (list: string[]) =>
    list.map((objectKey) => ({ objectKey, objectType: '' }))
  return listingWorld(
    typeof keys === 'function' ? () => typed(keys()) : typed(keys),
    pageSize,
  )
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
