import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { WatchBillingStateResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface BillingStateContextValue {
  // billingAccountId is the watched billing account id, if any.
  billingAccountId?: string
  // selfServiceAllowed indicates the viewer can directly manage the subscription.
  selfServiceAllowed: boolean
  // response is the current watched billing snapshot.
  response: WatchBillingStateResponse | null
  // loading indicates the billing snapshot has not emitted yet.
  loading: boolean
}

const Context = createContext<BillingStateContextValue | null>(null)

// BillingStateProvider watches a billing snapshot and exposes it via context.
export function BillingStateProvider(props: {
  billingAccountId?: string
  children?: ReactNode
}) {
  const sessionResource = SessionContext.useContext()
  const billingAccountId = props.billingAccountId
  const resource = useStreamingResource(
    sessionResource,
    useCallback(
      (session: NonNullable<Session>, signal: AbortSignal) =>
        session.spacewave.watchBillingState(billingAccountId, signal),
      [billingAccountId],
    ),
    [billingAccountId],
  )

  const value = useMemo(
    () => ({
      billingAccountId,
      selfServiceAllowed: !!billingAccountId,
      response: resource.value ?? null,
      loading: resource.loading && !resource.value,
    }),
    [billingAccountId, resource.loading, resource.value],
  )

  return <Context.Provider value={value}>{props.children}</Context.Provider>
}

// useBillingStateContext returns the current billing snapshot context.
export function useBillingStateContext(): BillingStateContextValue {
  const context = useContext(Context)
  if (!context) {
    throw new Error(
      'Billing state context not found. Wrap component in BillingStateProvider.',
    )
  }
  return context
}

// useBillingStateContextSafe returns the billing snapshot context or null.
export function useBillingStateContextSafe(): BillingStateContextValue | null {
  return useContext(Context)
}
