// contexts provides pre-created resource contexts for Root, Session, and Provider resources.
import { createContext, useContext } from 'react'

import { Root } from '@s4wave/sdk/root'
import { Provider } from '@s4wave/sdk/provider'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { createResourceContext } from '@aptre/bldr-sdk/hooks/createResourceContext.js'

import type { To } from '@s4wave/web/router/router.js'

// Re-export SessionContext and SessionIndexContext from their lightweight modules.
export { SessionContext } from './SessionContext.js'
export { SessionIndexContext, useSessionIndex } from './SessionIndexContext.js'

// RootContext provides the Root resource to child components.
export const RootContext = createResourceContext<Root>()

// ProviderContext provides the Provider resource to child components.
export const ProviderContext = createResourceContext<Provider>()

// SpaceContext provides the Space resource to child components.
export const SpaceContext = createResourceContext<Space>()

// SpaceContentsContext provides the SpaceContents resource to child components.
export const SpaceContentsContext = createResourceContext<SpaceContents>()

// SharedObjectContext provides the SharedObject resource to child components.
export const SharedObjectContext = createResourceContext<SharedObject>()

// SharedObjectBodyContext provides the SharedObjectBody resource to child components.
export const SharedObjectBodyContext = createResourceContext<SharedObjectBody>()

// SessionRouteContext provides the current session base path and a navigator
// rooted at that session, so children can target session URLs without
// rebuilding /u/:sessionIndex/... strings.
export const SessionRouteContext = createContext<{
  basePath: string
  navigate: (to: To) => void
} | null>(null)

function useSessionRouteContext(): {
  basePath: string
  navigate: (to: To) => void
} {
  const ctx = useContext(SessionRouteContext)
  if (!ctx) {
    throw new Error(
      'useSessionRouteContext must be used within SessionRouteContext',
    )
  }
  return ctx
}

// useSessionNavigate returns a navigate function rooted at the current session.
export function useSessionNavigate(): (to: To) => void {
  return useSessionRouteContext().navigate
}
