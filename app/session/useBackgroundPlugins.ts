import { useCallback, useMemo } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { BackgroundPlugin } from '@s4wave/core/session/session.pb.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionIndex } from '@s4wave/web/contexts/SessionIndexContext.js'

// BackgroundPlugins is the Session's saved background plugin choices.
export interface BackgroundPlugins {
  // entries lists each confirmed plugin by Space, running or suspended.
  entries: BackgroundPlugin[]
  // set confirms, suspends, or withdraws one plugin of one Space.
  set: (
    spaceId: string,
    pluginId: string,
    enabled: boolean,
    suspended: boolean,
  ) => Promise<void>
}

// useBackgroundPlugins streams the current Session's background plugin
// choices and returns the setter that saves a new choice.
export function useBackgroundPlugins(): BackgroundPlugins {
  const sessionIndex = useSessionIndex()
  const metadata = useSessionMetadata(sessionIndex)
  const session = useResourceValue(SessionContext.useContext())
  const set = useCallback(
    async (
      spaceId: string,
      pluginId: string,
      enabled: boolean,
      suspended: boolean,
    ) => {
      if (!session) throw new Error('session is not ready')
      await session.setBackgroundPlugin(spaceId, pluginId, enabled, suspended)
    },
    [session],
  )
  const entries = metadata?.backgroundPlugins
  return useMemo(() => ({ entries: entries ?? [], set }), [entries, set])
}
