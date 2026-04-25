import {
  AccountLifecycleState,
  BillingInterval,
  BillingStatus,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// statusLabel returns a human-readable label for a billing status.
export function statusLabel(status?: BillingStatus): string {
  switch (status) {
    case BillingStatus.BillingStatus_ACTIVE:
      return 'Active'
    case BillingStatus.BillingStatus_TRIALING:
      return 'Trial'
    case BillingStatus.BillingStatus_PAST_DUE:
    case BillingStatus.BillingStatus_PAST_DUE_READONLY:
      return 'Past Due'
    case BillingStatus.BillingStatus_CANCELED:
      return 'Canceled'
    case BillingStatus.BillingStatus_NONE:
      return 'No Subscription'
    default:
      return 'Unknown'
  }
}

// intervalLabel returns a human-readable label for a billing interval.
export function intervalLabel(interval?: BillingInterval): string {
  switch (interval) {
    case BillingInterval.BillingInterval_MONTH:
      return 'Monthly'
    case BillingInterval.BillingInterval_YEAR:
      return 'Annual'
    default:
      return ''
  }
}

// isStatusActive returns true for active or trialing statuses.
export function isStatusActive(status?: BillingStatus): boolean {
  return (
    status === BillingStatus.BillingStatus_ACTIVE ||
    status === BillingStatus.BillingStatus_TRIALING
  )
}

// isStatusPastDue returns true for past-due statuses.
export function isStatusPastDue(status?: BillingStatus): boolean {
  return (
    status === BillingStatus.BillingStatus_PAST_DUE ||
    status === BillingStatus.BillingStatus_PAST_DUE_READONLY
  )
}

// deleteBillingAccountDisabledReason explains why a billing account cannot be
// deleted yet.
export function deleteBillingAccountDisabledReason(
  status: BillingStatus | undefined,
  assigneeCount: number,
): string | null {
  if (isStatusPastDue(status)) {
    return 'This billing account still has a past-due balance. Resolve the balance before deleting it.'
  }
  if (
    status !== BillingStatus.BillingStatus_CANCELED &&
    status !== BillingStatus.BillingStatus_NONE
  ) {
    return 'Only canceled billing accounts or billing accounts with no subscription can be deleted.'
  }
  if (assigneeCount > 0) {
    return 'Detach this billing account from every personal account and organization before deleting it.'
  }
  return null
}

// subscriptionStatusLabel returns a human-readable label for a billing status.
export function subscriptionStatusLabel(status?: BillingStatus): string {
  switch (status) {
    case BillingStatus.BillingStatus_ACTIVE:
      return 'Active'
    case BillingStatus.BillingStatus_TRIALING:
      return 'Trial'
    case BillingStatus.BillingStatus_PAST_DUE:
    case BillingStatus.BillingStatus_PAST_DUE_READONLY:
      return 'Past due'
    case BillingStatus.BillingStatus_CANCELED:
      return 'Canceled'
    case BillingStatus.BillingStatus_LAPSED:
      return 'Lapsed'
    case BillingStatus.BillingStatus_DELETED:
      return 'Deleted'
    case BillingStatus.BillingStatus_NONE:
      return 'No subscription'
    default:
      return 'Unknown'
  }
}

// lifecycleStateLabel returns a human-readable label for a billing account
// lifecycle state.
export function lifecycleStateLabel(state?: AccountLifecycleState): string {
  switch (state) {
    case AccountLifecycleState.AccountLifecycleState_ACTIVE:
      return 'Active'
    case AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END:
      return 'Cancels at period end'
    case AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY:
      return 'Grace period (read-only)'
    case AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY:
      return 'Pending deletion'
    case AccountLifecycleState.AccountLifecycleState_LAPSED_READONLY:
      return 'Lapsed'
    case AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE:
      return 'Deleted (pending purge)'
    case AccountLifecycleState.AccountLifecycleState_DELETED:
      return 'Deleted'
    case AccountLifecycleState.AccountLifecycleState_DISPUTED_HARD_SUSPEND:
      return 'Suspended (dispute)'
    case undefined:
      return ''
    default:
      return 'Unknown'
  }
}

// subscriptionStatusBadgeColor returns a tailwind class pair for the badge
// chip on a billing status.
export function subscriptionStatusBadgeColor(status?: BillingStatus): string {
  switch (status) {
    case BillingStatus.BillingStatus_ACTIVE:
    case BillingStatus.BillingStatus_TRIALING:
      return 'bg-green-500/15 text-green-500'
    case BillingStatus.BillingStatus_PAST_DUE:
    case BillingStatus.BillingStatus_PAST_DUE_READONLY:
      return 'bg-yellow-500/15 text-yellow-500'
    case BillingStatus.BillingStatus_CANCELED:
    case BillingStatus.BillingStatus_LAPSED:
    case BillingStatus.BillingStatus_DELETED:
      return 'bg-destructive/15 text-destructive'
    default:
      return 'bg-foreground/10 text-foreground-alt/70'
  }
}
