import React, {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import type {
  OrganizationInfo,
  WatchOrganizationsResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface SpacewaveOrgListContextValue {
  // organizations is the current org list, or empty if not yet loaded.
  organizations: OrganizationInfo[]
  // loading indicates the org list has not been loaded yet.
  loading: boolean
}

const Context = createContext<SpacewaveOrgListContextValue | null>(null)

const Provider: React.FC<{
  response: WatchOrganizationsResponse | null
  loading: boolean
  children?: ReactNode
}> = ({ response, loading, children }) => {
  const value: SpacewaveOrgListContextValue = useMemo(
    () => ({
      organizations: response?.organizations ?? [],
      loading: loading && !response,
    }),
    [response, loading],
  )

  return <Context.Provider value={value}>{children}</Context.Provider>
}

const useSpacewaveOrgListContext = (): SpacewaveOrgListContextValue => {
  const context = useContext(Context)
  if (!context) {
    throw new Error(
      'SpacewaveOrgList context not found. Wrap component in SpacewaveOrgListContext.Provider.',
    )
  }
  return context
}

const useSpacewaveOrgListContextSafe =
  (): SpacewaveOrgListContextValue | null => {
    return useContext(Context)
  }

// SpacewaveOrgListContext provides the org list to spacewave session children.
export const SpacewaveOrgListContext = {
  Provider,
  useContext: useSpacewaveOrgListContext,
  useContextSafe: useSpacewaveOrgListContextSafe,
}
