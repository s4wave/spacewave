import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { WatchStorageBackendsResponse } from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

// useStorageBackends watches the account's storage backends and the Spaces
// placed on each. The resource errors for a session without a local account,
// which cannot hold storage backends.
export function useStorageBackends(): Resource<WatchStorageBackendsResponse> {
  const sessionResource = SessionContext.useContext()
  return useStreamingResource(
    sessionResource,
    (session, signal) => session.watchStorageBackends(signal),
    [],
  )
}
