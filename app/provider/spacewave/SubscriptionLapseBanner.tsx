import { useCallback, useState } from 'react'
import { LuTriangleAlert, LuArrowRight } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { findPersonalCanceledBillingAccount } from './resubscribe-target.js'

// formatDate formats a Unix timestamp in milliseconds to a human-readable date.
function formatDate(ms: bigint): string {
  const date = new Date(Number(ms))
  const months = [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
  ]
  return `${months[date.getUTCMonth()]} ${date.getUTCDate()}, ${date.getUTCFullYear()}`
}

// SubscriptionLapseBanner shows a two-stage prompt when a subscription is lapsing.
// Stage 1 (nudge): cancel_at is set but subscription is still active.
// Stage 2 (read-only): subscription is canceled, show migration wizard link.
// Reads from SpacewaveOnboardingContext instead of opening its own stream.
export function SubscriptionLapseBanner() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  const onboarding = ctx?.onboarding ?? null
  const isReadOnlyGrace = ctx?.isReadOnlyGrace ?? false
  const { accountId } = useSessionInfo(session)
  const navigate = useNavigate()
  const parentPaths = useParentPaths()
  const path = usePath()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const basePath = parentPaths[parentPaths.length - 1] ?? path
  const [error, setError] = useState<string | null>(null)
  const [resolving, setResolving] = useState(false)

  const handleMigrateClick = useCallback(() => {
    navigate({ path: `${basePath}/settings/migration` })
  }, [navigate, basePath])

  const handleResubscribeClick = useCallback(async () => {
    if (!session || resolving) return

    setOpenMenu?.('')
    setResolving(true)
    setError(null)
    try {
      const resp = await session.spacewave.listManagedBillingAccounts()
      const target = findPersonalCanceledBillingAccount(
        resp.accounts ?? [],
        accountId,
      )
      if (target?.id) {
        navigate({ path: `${basePath}/billing/${target.id}?reactivate=1` })
        return
      }
      navigate({ path: `${basePath}/plan/no-active` })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setResolving(false)
    }
  }, [accountId, basePath, navigate, resolving, session, setOpenMenu])

  if (!onboarding) return null

  const cancelAt = onboarding.cancelAt ?? 0n
  const lifecycleState = onboarding.lifecycleState

  // Stage 1: subscription active but cancel_at is set (pending cancellation).
  if (
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END &&
    cancelAt > 0n
  ) {
    return (
      <div className="border-warning/20 bg-warning/5 flex items-center gap-2 border-b px-3 py-1.5">
        <LuTriangleAlert className="text-warning h-3.5 w-3.5 shrink-0" />
        <p className="text-foreground/80 text-xs font-medium">
          Your subscription ends on {formatDate(cancelAt)}.
        </p>
      </div>
    )
  }

  // Lapsed or grace period: determine message and action.
  let message: string | undefined
  let actionLabel: string | undefined
  let onAction: (() => void) | undefined

  if (
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY
  ) {
    message =
      'Your subscription has ended. Cloud data is read-only for 30 days so you can export or re-subscribe.'
    actionLabel = 'Resubscribe'
    onAction = handleResubscribeClick
  } else if (
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_LAPSED_READONLY
  ) {
    message = 'Your cloud account is inactive until you re-subscribe.'
    actionLabel = 'Resubscribe'
    onAction = handleResubscribeClick
  } else if (
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE ||
    lifecycleState === AccountLifecycleState.AccountLifecycleState_DELETED
  ) {
    message = 'This cloud account has been deleted from the product.'
  } else if (isReadOnlyGrace) {
    message =
      'Your subscription has ended. Cloud data is read-only during the grace period.'
    actionLabel = 'Migrate to local'
    onAction = handleMigrateClick
  }

  if (!message) return null

  if (!onAction || !actionLabel) {
    return (
      <div className="border-destructive/20 bg-destructive/5 flex items-center border-b">
        <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
          <LuTriangleAlert className="text-destructive h-3.5 w-3.5 shrink-0" />
          <div className="min-w-0">
            <p className="text-foreground/80 text-xs font-medium">{message}</p>
            {error && (
              <p className="text-destructive mt-1 text-[11px]">{error}</p>
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={onAction}
      aria-disabled={resolving}
      className="border-destructive/20 bg-destructive/5 hover:bg-destructive/8 flex w-full items-center border-b text-left transition-colors disabled:cursor-default"
    >
      <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
        <LuTriangleAlert className="text-destructive h-3.5 w-3.5 shrink-0" />
        <div className="min-w-0">
          <p className="text-foreground/80 text-xs font-medium">{message}</p>
          {error && (
            <p className="text-destructive mt-1 text-[11px]">{error}</p>
          )}
        </div>
      </div>
      <div className="group flex shrink-0 items-center gap-1 px-3 py-1.5 transition-colors">
        <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
          {resolving ? 'Opening billing...' : actionLabel}
        </span>
        <LuArrowRight className="text-foreground-alt group-hover:text-foreground h-3 w-3 shrink-0 transition-colors" />
      </div>
    </button>
  )
}
