import { useCallback, useMemo } from 'react'

import type { GetSessionInfoResponse } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { usePromise } from './usePromise.js'

// SessionInfo contains derived provider fields from a session.
export interface SessionInfo {
  // sessionInfo is the raw GetSessionInfoResponse, or undefined while loading.
  sessionInfo: GetSessionInfoResponse | null | undefined
  // loading is true while the getSessionInfo RPC is in flight.
  loading: boolean
  // error is non-null if the getSessionInfo RPC failed.
  error: Error | null
  // providerId is the provider identifier (e.g. "spacewave", "local").
  providerId: string
  // accountId is the provider account identifier.
  accountId: string
  // peerId is the session's peer ID.
  peerId: string
  // isCloud is true when the session uses a remote provider (not local).
  isCloud: boolean
}

// useSessionInfo fetches session info and derives common provider fields.
export function useSessionInfo(
  session: Session | null | undefined,
): SessionInfo {
  const {
    data: sessionInfo,
    loading,
    error,
  } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.getSessionInfo(signal) ?? Promise.resolve(null),
      [session],
    ),
  )
  return useMemo(() => {
    const providerId =
      sessionInfo?.sessionRef?.providerResourceRef?.providerId ?? ''
    const accountId =
      sessionInfo?.sessionRef?.providerResourceRef?.providerAccountId ?? ''
    const peerId = sessionInfo?.peerId ?? ''
    const isCloud = providerId !== '' && providerId !== 'local'
    return {
      sessionInfo,
      loading,
      error,
      providerId,
      accountId,
      peerId,
      isCloud,
    }
  }, [sessionInfo, loading, error])
}
