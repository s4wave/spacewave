import type { DependencyList } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useStreamingResource,
  type UseStreamingResourceOptions,
} from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  watchWorldQuery,
  type WorldQuery,
} from '@s4wave/sdk/world/world-query.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

/** UseWorldQueryOptions configures a World query watch. */
export interface UseWorldQueryOptions<
  T,
> extends UseStreamingResourceOptions<T> {
  /** key shares one watch among the hooks on a World that pass it. */
  key?: string
  /**
   * share keeps every part of a new result that deep-equals the previous
   * one, so an unchanged result keeps its identity and a changed list keeps
   * its unchanged entries. See shareEqual for what it compares.
   */
  share?: boolean
}

/**
 * useWorldQuery runs a bounded World query and re-runs it on each World
 * revision. A live World is watched through its Engine; a World without one,
 * such as a historical snapshot, is read once. deps lists the values the
 * query closes over; changing one restarts the watch. A watch that fails
 * after its first result reconnects; options can override that or add an
 * equals check that keeps an unchanged result. Hooks on one World that pass
 * the same key share one watch, so the key must name everything the query
 * reads. With share set, a result keeps the parts that did not change.
 */
export function useWorldQuery<T>(
  world: Resource<IWorldState>,
  query: WorldQuery<T>,
  deps: DependencyList,
  options?: UseWorldQueryOptions<T>,
): Resource<T> {
  const { key, share, ...streamOptions } = options ?? {}
  return useStreamingResource(
    world,
    async function* (state, signal) {
      const engine = state.getEngine?.()
      if (!engine) {
        yield await query(state, signal)
        return
      }
      let prev: { value: T } | undefined
      for await (const snapshot of watchWorldQuery(
        engine,
        query,
        signal,
        key,
      )) {
        const value =
          share && prev
            ? shareEqual(prev.value, snapshot.value)
            : snapshot.value
        prev = { value }
        yield value
      }
    },
    deps,
    {
      reconnect: true,
      ...(share ? { equals: Object.is } : {}),
      ...streamOptions,
    },
  )
}

/**
 * shareEqual returns next with every part that deep-equals the matching part
 * of prev replaced by prev's, so an unchanged value keeps its identity and a
 * changed array or object keeps the identity of its unchanged entries. Array
 * entries match by index. It walks arrays, plain objects, Dates and byte
 * arrays; any other value, such as a class instance or a handle, equals only
 * itself.
 */
export function shareEqual<T>(prev: T, next: T): T {
  return share(prev, next) as T
}

// share returns prev when it deep-equals next, else next with its equal parts
// taken from prev.
function share(prev: unknown, next: unknown): unknown {
  if (Object.is(prev, next)) return prev
  if (prev instanceof Date && next instanceof Date) {
    return prev.getTime() === next.getTime() ? prev : next
  }
  if (prev instanceof Uint8Array && next instanceof Uint8Array) {
    return bytesEqual(prev, next) ? prev : next
  }
  if (Array.isArray(prev) && Array.isArray(next)) {
    const out = next.map((item, i) => share(prev[i], item))
    const same =
      out.length === prev.length && out.every((item, i) => item === prev[i])
    return same ? prev : out
  }
  if (isPlainObject(prev) && isPlainObject(next)) {
    const keys = Object.keys(next)
    let same = keys.length === Object.keys(prev).length
    const out: Record<string, unknown> = {}
    for (const key of keys) {
      out[key] = share(prev[key], next[key])
      if (out[key] !== prev[key] || !(key in prev)) same = false
    }
    return same ? prev : out
  }
  return next
}

// isPlainObject reports whether value is an object literal.
function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null) return false
  const proto = Object.getPrototypeOf(value) as unknown
  return proto === Object.prototype || proto === null
}

// bytesEqual reports whether a and b hold the same bytes.
function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}
