import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { WatchSpaceStorageResponse } from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

// useSpaceStorage watches where a Space's blocks are stored and the progress
// of their upload. The watch restarts when sharedObjectId changes.
export function useSpaceStorage(
  sharedObjectId: string,
): Resource<WatchSpaceStorageResponse> {
  const sessionResource = SessionContext.useContext()
  return useStreamingResource(
    sessionResource,
    (session, signal) => session.watchSpaceStorage(sharedObjectId, signal),
    [sharedObjectId],
  )
}
