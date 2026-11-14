import { useCallback, useMemo, useState } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import { isReleasedHandle } from '@aptre/bldr-sdk/hooks/resourceTransitionState.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { Resource as SDKResource } from '@aptre/bldr-sdk/resource/resource.js'
import { TypedObjectResourceServiceClient } from '@s4wave/sdk/world/world_srpc.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// TypedHandle is the availability of a World object's typed handler.
export type TypedHandle<T> =
  // The object is absent, has no type, or has another type than requested.
  | { state: 'absent' }
  // The object has the type, but no handler serves it on this Device yet.
  | { state: 'no-handler' }
  // The handler serves the object through handle.
  | { state: 'ready'; handle: T }

// useWatchTypedHandle follows the typed handler of a World object. Unlike
// useAccessTypedHandle it does not wait for a handler: it reports one missing,
// then switches to ready when a handler starts, and back when it stops. Each
// handle is released when the next state replaces it. A failed watch keeps no
// last state: the watch released its handle as it ended.
export function useWatchTypedHandle<T extends SDKResource>(
  worldState: Resource<IWorldState>,
  objectKey: string,
  HandleClass: new (ref: ClientResourceRef) => T,
  typeId: string,
): Resource<TypedHandle<T>> {
  const watch = useStreamingResource(
    worldState,
    useCallback(
      (world: IWorldState, signal: AbortSignal) =>
        watchTypedHandle(world, objectKey, HandleClass, typeId, signal),
      [objectKey, HandleClass, typeId],
    ),
    [objectKey, HandleClass, typeId],
    { reconnect: true },
  )
  return useMemo(
    () => (watch.error ? resourceOf({ ...watch, last: null }) : watch),
    [watch],
  )
}

/** HeldTypedHandle is the handle a viewer draws its body on. */
export type HeldTypedHandle<T> = {
  handle: T
  // live is set while handle is the watch's ready handle and not released.
  // Otherwise it is the last ready handle, already released, as it is while
  // the watch fails, reloads, or waits to reconnect still showing its ready
  // state: the body reads it only through watches that report the loss, runs
  // no query on it, and offers no write.
  live: boolean
}

/**
 * useHeldTypedHandle returns the handle a viewer draws its body on: the
 * ready one, or while the watch has failed or reloads after a failure the
 * last one that was ready for objectKey, so the body and the drafts in it
 * stay mounted until the next handle arrives. A held handle never crosses
 * to another object. It is null while the object is absent, has no
 * handler, or has not been ready.
 */
export function useHeldTypedHandle<T>(
  handle: Resource<TypedHandle<T>>,
  objectKey: string,
): HeldTypedHandle<T> | null {
  // Take the watch's ready handle, else the last one held for objectKey
  // while the watch fails or reloads.
  const value = handle.value
  const ready = value?.state === 'ready' ? value.handle : null
  const [last, setLast] = useState<{ key: string; handle: T } | null>(null)
  const kept =
    value === null &&
    (handle.error || handle.loading) &&
    last?.key === objectKey
      ? last.handle
      : null
  const held = ready ?? kept

  // Remember the held handle for the next render.
  if (held !== (last?.handle ?? null) || (held && last?.key !== objectKey)) {
    setLast(held ? { key: objectKey, handle: held } : null)
  }

  // A released handle is held but never live.
  return held
    ? { handle: held, live: held === ready && !isReleasedHandle(held) }
    : null
}

/**
 * useLiveDeps returns deps while live, otherwise the deps of the last live
 * render, so a resource keyed on them runs no query on a held handle and
 * keeps its answer until the next handle arrives. A resource first mounted
 * while not live still runs once; its factory returns null then.
 */
export function useLiveDeps(
  live: boolean,
  deps: React.DependencyList,
): React.DependencyList {
  const [last, setLast] = useState(deps)
  if (live && !sameDeps(last, deps)) setLast(deps)
  return live ? deps : last
}

// sameDeps reports whether a and b hold the same values, as React compares
// a dependency list.
function sameDeps(a: React.DependencyList, b: React.DependencyList): boolean {
  return a.length === b.length && a.every((v, i) => Object.is(v, b[i]))
}

// watchTypedHandle yields the typed handler state of objectKey until signal
// aborts, releasing each handle once it is replaced or the watch ends.
async function* watchTypedHandle<T extends SDKResource>(
  world: IWorldState,
  objectKey: string,
  HandleClass: new (ref: ClientResourceRef) => T,
  typeId: string,
  signal: AbortSignal,
): AsyncIterable<TypedHandle<T>> {
  const resourceRef = world.getResourceRef()
  const service = new TypedObjectResourceServiceClient(resourceRef.client)
  let handle: T | null = null
  try {
    for await (const resp of service.WatchTypedObject({ objectKey }, signal)) {
      // Release the previous generation's handle.
      handle?.release()
      handle = null

      // Map the response to a state.
      if (resp.typeId !== typeId) {
        yield { state: 'absent' }
        continue
      }
      if (!resp.resourceId) {
        yield { state: 'no-handler' }
        continue
      }
      handle = new HandleClass(resourceRef.createRef(resp.resourceId))
      yield { state: 'ready', handle }
    }
  } finally {
    handle?.release()
  }
}
