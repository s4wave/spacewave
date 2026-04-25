import { useCallback } from 'react'

import type { WatchOrganizationsResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

// useWatchOrganizations streams the current user's org list, updating live.
export function useWatchOrganizations(): Resource<WatchOrganizationsResponse> {
  const sessionResource = SessionContext.useContext()
  return useStreamingResource(
    sessionResource,
    useCallback(
      (session: NonNullable<Session>, signal: AbortSignal) =>
        session.spacewave.watchOrganizations(signal),
      [],
    ),
    [],
  )
}
