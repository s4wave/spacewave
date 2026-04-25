import { useCallback } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import {
  WatchControllersRequest,
  WatchControllersResponse,
  WatchDirectivesRequest,
  WatchDirectivesResponse,
} from '@s4wave/sdk/status/status.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import {
  WatchResourcesListRequest as SessionWatchResourcesListRequest,
  WatchResourcesListResponse as SessionWatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'

// useWatchControllers streams the list of active controllers from the session.
// Returns the latest WatchControllersResponse or null while loading.
export function useWatchControllers(): WatchControllersResponse | null {
  const session = SessionContext.useContext()
  const sessionValue = useResourceValue(session)

  const watchFn = useCallback(
    (_: WatchControllersRequest, signal: AbortSignal) =>
      sessionValue?.systemStatus.watchControllers(signal) ?? null,
    [sessionValue],
  )

  return useWatchStateRpc(
    watchFn,
    {},
    WatchControllersRequest.equals,
    WatchControllersResponse.equals,
  )
}

// useWatchDirectives streams the list of active directives from the session.
// Returns the latest WatchDirectivesResponse or null while loading.
export function useWatchDirectives(): WatchDirectivesResponse | null {
  const session = SessionContext.useContext()
  const sessionValue = useResourceValue(session)

  const watchFn = useCallback(
    (_: WatchDirectivesRequest, signal: AbortSignal) =>
      sessionValue?.systemStatus.watchDirectives(signal) ?? null,
    [sessionValue],
  )

  return useWatchStateRpc(
    watchFn,
    {},
    WatchDirectivesRequest.equals,
    WatchDirectivesResponse.equals,
  )
}

// useWatchSpacesList streams the session's current space list from the
// existing WatchResourcesList RPC. Returns null while loading.
export function useWatchSpacesList(): ReadonlyArray<SpaceSoListEntry> | null {
  const session = SessionContext.useContext()
  const sessionValue = useResourceValue(session)

  const watchFn = useCallback(
    (_: SessionWatchResourcesListRequest, signal: AbortSignal) =>
      sessionValue?.watchResourcesList({}, signal) ?? null,
    [sessionValue],
  )

  const resp = useWatchStateRpc(
    watchFn,
    {},
    SessionWatchResourcesListRequest.equals,
    SessionWatchResourcesListResponse.equals,
  )

  return resp?.spacesList ?? null
}
