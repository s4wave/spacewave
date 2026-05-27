import { createContext, use, useMemo, type ReactNode } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { WatchStorageStatsResponse } from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

export const setupBannerStorageThresholdBytes = 10n * 1024n * 1024n

export interface SessionStorageStatsView {
  snapshot: WatchStorageStatsResponse | null
  loading: boolean
  supported: boolean
  totalBytes: bigint
  blockCount: bigint
  setupBannerEligible: boolean
}

const loadingView: SessionStorageStatsView = {
  snapshot: null,
  loading: true,
  supported: false,
  totalBytes: 0n,
  blockCount: 0n,
  setupBannerEligible: false,
}

const SessionStorageStatsContext =
  createContext<SessionStorageStatsView>(loadingView)

// SessionStorageStatsProvider owns the session storage-stats watch for a session UI tree.
export function SessionStorageStatsProvider({
  children,
}: {
  children: ReactNode
}) {
  const sessionResource = SessionContext.useContext()
  const resource = useStreamingResource(
    sessionResource,
    (session, signal) => session.watchStorageStats({}, signal),
    [],
  )
  const value = useMemo(
    () =>
      buildSessionStorageStatsView(
        resource?.value ?? null,
        resource?.loading ?? true,
      ),
    [resource?.loading, resource?.value],
  )

  return (
    <SessionStorageStatsContext.Provider value={value}>
      {children}
    </SessionStorageStatsContext.Provider>
  )
}

// useSessionStorageStats returns the current provider-owned session storage stats view.
export function useSessionStorageStats(): SessionStorageStatsView {
  return use(SessionStorageStatsContext)
}

export function buildSessionStorageStatsView(
  snapshot: WatchStorageStatsResponse | null,
  loading: boolean,
): SessionStorageStatsView {
  if (loading && !snapshot) {
    return loadingView
  }

  const supported = snapshot?.supported ?? false
  const totalBytes = snapshot?.totalBytes ?? 0n
  const blockCount = snapshot?.blockCount ?? 0n
  return {
    snapshot,
    loading: false,
    supported,
    totalBytes,
    blockCount,
    setupBannerEligible:
      supported && totalBytes > setupBannerStorageThresholdBytes,
  }
}
