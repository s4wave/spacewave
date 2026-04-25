import { useCallback, useMemo } from 'react'

import { useWatchStateRpc } from '@aptre/bldr-react'

import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import {
  WatchAllAccountStatusesRequest,
  WatchAllAccountStatusesResponse,
} from '@s4wave/sdk/root/root.pb.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'

// useSessionAccountStatuses streams account status for all configured sessions.
export function useSessionAccountStatuses(): Map<
  number,
  ProviderAccountStatus
> {
  const root = useRootResource().value
  const resp = useWatchStateRpc(
    useCallback(
      (req: WatchAllAccountStatusesRequest, signal: AbortSignal) => {
        if (!root) return null
        return root.watchAllAccountStatuses(req, signal)
      },
      [root],
    ),
    {},
    WatchAllAccountStatusesRequest.equals,
    WatchAllAccountStatusesResponse.equals,
  )

  return useMemo(() => {
    const statuses = new Map<number, ProviderAccountStatus>()
    for (const row of resp?.statuses ?? []) {
      statuses.set(
        row.sessionIdx ?? 0,
        row.accountStatus ?? ProviderAccountStatus.ProviderAccountStatus_NONE,
      )
    }
    return statuses
  }, [resp])
}
