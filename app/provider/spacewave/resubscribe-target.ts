import {
  AccountLifecycleState,
  BillingStatus,
  type ManagedBillingAccount,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// findPersonalCanceledBillingAccount returns the caller's personal canceled BA.
export function findPersonalCanceledBillingAccount(
  accounts: ManagedBillingAccount[],
  accountId: string | null,
): ManagedBillingAccount | null {
  if (!accountId) return null

  return (
    accounts.find((ba) => {
      const assignedToCaller = (ba.assignees ?? []).some(
        (a) => a.ownerType === 'account' && a.ownerId === accountId,
      )
      if (!assignedToCaller) return false

      const status = ba.subscriptionStatus
      const state = ba.lifecycleState
      return (
        status === BillingStatus.BillingStatus_CANCELED ||
        state ===
          AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY ||
        state === AccountLifecycleState.AccountLifecycleState_LAPSED_READONLY
      )
    }) ?? null
  )
}
