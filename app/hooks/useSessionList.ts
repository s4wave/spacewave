import { useEffect, useMemo } from 'react'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import type { WatchSessionsResponse } from '@s4wave/sdk/root/root.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import {
  SessionListState,
  type SessionListEntry,
  type SessionMetadata,
} from '../../core/session/session.pb.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'

// EMPTY_SESSIONS is the static-mode fallback for useSessionList.
const EMPTY_SESSIONS: Resource<WatchSessionsResponse> = {
  loading: false,
  value: { sessions: [] },
  error: null,
  retry: () => {},
}

// isSessionOpenable reports whether a session list entry opens. Only an
// ACTIVE entry does: a DELETING or UNLINKING entry belongs to an account whose
// delete has started, and the next startup finishes it.
export function isSessionOpenable(entry: SessionListEntry): boolean {
  return (entry.state ?? SessionListState.ACTIVE) === SessionListState.ACTIVE
}

// useSessionList returns the sessions that open, updating live. Every caller
// that routes to or opens a session reads this list.
export function useSessionList(): Resource<WatchSessionsResponse> {
  const resource = useAllSessionList()
  const value = resource.value
  const openable = useMemo(
    () =>
      value
        ? {
            ...value,
            sessions: (value.sessions ?? []).filter(isSessionOpenable),
          }
        : value,
    [value],
  )
  // The filter keeps value null exactly when the resource's is, so the
  // resource stays in the state it was in.
  return useMemo(
    () => ({ ...resource, value: openable }) as Resource<WatchSessionsResponse>,
    [resource, openable],
  )
}

// useAllSessionList returns every listed session, including those of an
// account whose delete has started, updating live. Only a view that shows
// such sessions without opening them reads it.
export function useAllSessionList(): Resource<WatchSessionsResponse> {
  const environment = useAppEnvironment()
  const isStatic = useIsStaticMode()
  const rootResource = useRootResource()
  const resource = useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) => root.watchSessions({}, signal),
    [],
  )
  useEffect(() => {
    if (isStatic) return
    if (resource.loading) return
    const hasSessions = (resource.value?.sessions ?? []).some(isSessionOpenable)
    if (hasSessions) {
      environment.storage.setItem('spacewave-has-session', '1')
      return
    }
    environment.storage.removeItem('spacewave-has-session')
  }, [isStatic, resource.loading, resource.value, environment])
  if (isStatic) return EMPTY_SESSIONS
  return resource
}

// SessionWithMeta pairs a session list entry with its optional metadata.
export interface SessionWithMeta {
  entry: SessionListEntry
  metadata?: SessionMetadata
}

// sortSessionsNewestFirst sorts sessions by created_at descending.
// Falls back to session_index descending when created_at is zero or missing.
export function sortSessionsNewestFirst(
  sessions: SessionWithMeta[],
): SessionWithMeta[] {
  return sessions.toSorted((a, b) => {
    const aTime = Number(a.metadata?.createdAt ?? 0n)
    const bTime = Number(b.metadata?.createdAt ?? 0n)
    if (aTime !== 0 && bTime !== 0) {
      return bTime - aTime
    }
    // Both zero or one zero: fall back to session_index descending.
    if (aTime !== bTime) {
      // Non-zero sorts before zero (newer).
      return aTime === 0 ? 1 : -1
    }
    return (b.entry.sessionIndex ?? 0) - (a.entry.sessionIndex ?? 0)
  })
}
