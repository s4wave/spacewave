import React, {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import {
  type WatchOnboardingStatusResponse,
  BillingStatus,
  AccountLifecycleState,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface SpacewaveOnboardingContextValue {
  // onboarding is the current onboarding status, or null if not yet loaded.
  onboarding: WatchOnboardingStatusResponse | null
  // isLapsed indicates a lapsed subscription (canceled, past_due_readonly, deleted).
  isLapsed: boolean
  // hasActiveBilling indicates the subscription status is active or trialing.
  // It reflects billing-subscription state only and does NOT infer activity
  // from the account lifecycle state.
  hasActiveBilling: boolean
  // isReadOnlyGrace indicates the account is in a product-visible read-only state.
  isReadOnlyGrace: boolean
  // isPendingDelete indicates the account is in the 24-hour delete-now countdown.
  isPendingDelete: boolean
  // isFunctionallyDeleted indicates the account is logically deleted.
  isFunctionallyDeleted: boolean
  // emailVerified indicates the account has at least one verified email.
  emailVerified: boolean
}

const Context = createContext<SpacewaveOnboardingContextValue | null>(null)

const Provider: React.FC<{
  onboarding: WatchOnboardingStatusResponse | null
  children?: ReactNode
}> = ({ onboarding, children }) => {
  const value: SpacewaveOnboardingContextValue = useMemo(() => {
    const status = onboarding?.subscriptionStatus
    const lifecycleState = onboarding?.lifecycleState
    const isPendingDelete =
      lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY
    const isReadOnlyGrace =
      lifecycleState ===
        AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY ||
      lifecycleState ===
        AccountLifecycleState.AccountLifecycleState_LAPSED_READONLY ||
      status === BillingStatus.BillingStatus_CANCELED ||
      status === BillingStatus.BillingStatus_PAST_DUE_READONLY ||
      status === BillingStatus.BillingStatus_LAPSED
    const isFunctionallyDeleted =
      lifecycleState ===
        AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE ||
      lifecycleState === AccountLifecycleState.AccountLifecycleState_DELETED ||
      status === BillingStatus.BillingStatus_DELETED
    return {
      onboarding,
      isLapsed: isPendingDelete || isReadOnlyGrace || isFunctionallyDeleted,
      hasActiveBilling:
        status === BillingStatus.BillingStatus_ACTIVE ||
        status === BillingStatus.BillingStatus_TRIALING,
      isPendingDelete,
      isReadOnlyGrace,
      isFunctionallyDeleted,
      emailVerified: !!onboarding?.emailVerified,
    }
  }, [onboarding])

  return <Context.Provider value={value}>{children}</Context.Provider>
}

const useSpacewaveOnboardingContext = (): SpacewaveOnboardingContextValue => {
  const context = useContext(Context)
  if (!context) {
    throw new Error(
      'SpacewaveOnboarding context not found. Wrap component in SpacewaveOnboardingContext.Provider.',
    )
  }
  return context
}

// useSpacewaveOnboardingContextSafe returns the context value or null if not available.
const useSpacewaveOnboardingContextSafe =
  (): SpacewaveOnboardingContextValue | null => {
    return useContext(Context)
  }

// SpacewaveOnboardingContext provides onboarding state to spacewave session children.
export const SpacewaveOnboardingContext = {
  Provider,
  useContext: useSpacewaveOnboardingContext,
  useContextSafe: useSpacewaveOnboardingContextSafe,
}
