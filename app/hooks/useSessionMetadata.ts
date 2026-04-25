import { useCallback, useMemo } from 'react'

import { useWatchStateRpc } from '@aptre/bldr-react'

import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import {
  WatchSessionMetadataRequest,
  WatchSessionMetadataResponse,
} from '@s4wave/sdk/root/root.pb.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'

// useSessionMetadata streams metadata for a session index.
export function useSessionMetadata(
  sessionIdx: number | null,
): SessionMetadata | null {
  const root = useRootResource().value
  const req = useMemo(
    () => (sessionIdx == null ? null : { sessionIdx }),
    [sessionIdx],
  )
  const resp = useWatchStateRpc(
    useCallback(
      (req: WatchSessionMetadataRequest, signal: AbortSignal) => {
        if (!root || sessionIdx == null) return null
        return root.watchSessionMetadata(req, signal)
      },
      [root, sessionIdx],
    ),
    req,
    WatchSessionMetadataRequest.equals,
    WatchSessionMetadataResponse.equals,
  )

  if (sessionIdx == null || resp?.notFound) {
    return null
  }
  return resp?.metadata ?? null
}
