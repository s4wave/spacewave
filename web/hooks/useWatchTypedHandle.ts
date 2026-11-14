import { useCallback, useMemo, useState } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
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

/**
 * useHeldTypedHandle returns the handle a viewer draws its body on: the
 * ready one, or while the watch has failed the last one that was ready, so
 * the body and the drafts in it stay mounted under the failure. A held
 * handle is released; the body reads it only through watches that report
 * the watch's failure, and offers no write on it. It is null while the
 * object is absent, has no handler, or has not been ready.
 */
export function useHeldTypedHandle<T>(
  handle: Resource<TypedHandle<T>>,
): T | null {
  const value = handle.value
  const ready = value?.state === 'ready' ? value.handle : null
  const [last, setLast] = useState<T | null>(null)
  const held = ready ?? (handle.error ? last : null)
  if (held !== last) setLast(held)
  return held
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
