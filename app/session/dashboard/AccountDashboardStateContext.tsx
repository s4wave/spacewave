import { createContext, useContext, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type {
  WatchAccountInfoResponse,
  WatchAuthMethodsResponse,
  WatchEntityKeypairsResponse,
} from '@s4wave/sdk/account/account.pb.js'

export interface AccountDashboardState {
  account: Resource<Account>
  accountInfo: Resource<WatchAccountInfoResponse>
  authMethods: Resource<WatchAuthMethodsResponse>
  entityKeypairs: Resource<WatchEntityKeypairsResponse>
}

export interface AccountDashboardStateProviderProps {
  account: Resource<Account>
  children: React.ReactNode
}

const AccountDashboardStateContext =
  createContext<AccountDashboardState | null>(null)

// AccountDashboardStateProvider owns shared account dashboard watch streams.
export function AccountDashboardStateProvider({
  account,
  children,
}: AccountDashboardStateProviderProps) {
  const accountInfo = useStreamingResource(
    account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )
  const authMethods = useStreamingResource(
    account,
    (acc, signal) => acc.watchAuthMethods({}, signal),
    [],
  )
  const entityKeypairs = useStreamingResource(
    account,
    (acc, signal) => acc.watchEntityKeypairs({}, signal),
    [],
  )

  const value = useMemo(
    () => ({
      account,
      accountInfo,
      authMethods,
      entityKeypairs,
    }),
    [account, accountInfo, authMethods, entityKeypairs],
  )

  return (
    <AccountDashboardStateContext.Provider value={value}>
      {children}
    </AccountDashboardStateContext.Provider>
  )
}

// useAccountDashboardState returns shared dashboard watches for an account.
export function useAccountDashboardState(
  account: Resource<Account> | null | undefined,
): AccountDashboardState | null {
  const state = useContext(AccountDashboardStateContext)
  if (!account || state?.account !== account) {
    return null
  }
  return state
}
