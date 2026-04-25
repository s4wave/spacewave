import { useEffect } from 'react'
import type { WatchSessionsResponse } from '@s4wave/sdk/root/root.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type {
  SessionListEntry,
  SessionMetadata,
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

// useSessionList returns the list of configured sessions, updating live.
export function useSessionList(): Resource<WatchSessionsResponse> {
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
    const hasSessions = (resource.value?.sessions?.length ?? 0) > 0
    if (hasSessions) {
      localStorage.setItem('spacewave-has-session', '1')
      return
    }
    localStorage.removeItem('spacewave-has-session')
  }, [isStatic, resource.loading, resource.value])
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
  return [...sessions].sort((a, b) => {
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
