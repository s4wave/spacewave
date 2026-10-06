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

type LapseAction = 'resubscribe' | 'migrate'

interface LapseNotice {
  message: string
  action?: LapseAction
  actionLabel?: string
}

/** lapseNotice returns the banner message and action for a lapsed or grace-period account. */
function lapseNotice(
  lifecycleState: AccountLifecycleState | undefined,
  isReadOnlyGrace: boolean,
): LapseNotice | null {
  switch (lifecycleState) {
    case AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY:
      return {
        message:
          'Your subscription has ended. Cloud data is read-only for 30 days so you can export or re-subscribe.',
        action: 'resubscribe',
        actionLabel: 'Resubscribe',
      }
    case AccountLifecycleState.AccountLifecycleState_LAPSED_READONLY:
      return {
        message: 'Your cloud account is inactive until you re-subscribe.',
        action: 'resubscribe',
        actionLabel: 'Resubscribe',
      }
    case AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE:
    case AccountLifecycleState.AccountLifecycleState_DELETED:
      return {
        message: 'This cloud account has been deleted from the product.',
      }
  }
  if (!isReadOnlyGrace) return null
  return {
    message:
      'Your subscription has ended. Cloud data is read-only during the grace period.',
    action: 'migrate',
    actionLabel: 'Migrate to local',
  }
}

/**
 * useLapseActions resolves the resubscribe target for the personal canceled
 * billing account and exposes the migrate shortcut.
 */
function useLapseActions() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { accountId } = useSessionInfo(session)
  const navigate = useNavigate()
  const parentPaths = useParentPaths()
  const path = usePath()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const basePath = parentPaths[parentPaths.length - 1] ?? path
  const [error, setError] = useState<string | null>(null)
  const [resolving, setResolving] = useState(false)

  const migrate = useCallback(() => {
    navigate({ path: `${basePath}/settings/migration` })
  }, [navigate, basePath])

  const resubscribe = useCallback(async () => {
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

  return { error, resolving, migrate, resubscribe }
}

interface LapseMessageProps {
  message: string
  error: string | null
}

/** LapseMessage renders the warning icon, the notice, and any action error. */
function LapseMessage({ message, error }: LapseMessageProps) {
  return (
    <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
      <LuTriangleAlert className="text-destructive size-3.5 shrink-0" />
      <div className="min-w-0">
        <p className="text-foreground/80 text-xs font-medium">{message}</p>
        {error && <p className="text-destructive mt-1 text-xs">{error}</p>}
      </div>
    </div>
  )
}

// SubscriptionLapseBanner shows a two-stage prompt when a subscription is lapsing.
// Stage 1 (nudge): cancel_at is set but subscription is still active.
// Stage 2 (read-only): subscription is canceled, show migration wizard link.
// Reads from SpacewaveOnboardingContext instead of opening its own stream.
export function SubscriptionLapseBanner() {
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  const onboarding = ctx?.onboarding ?? null
  const isReadOnlyGrace = ctx?.isReadOnlyGrace ?? false
  const { error, resolving, migrate, resubscribe } = useLapseActions()

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
        <LuTriangleAlert className="text-warning size-3.5 shrink-0" />
        <p className="text-foreground/80 text-xs font-medium">
          Your subscription ends on {formatDate(cancelAt)}.
        </p>
      </div>
    )
  }

  // Lapsed or grace period: show the notice, with its action when it has one.
  const notice = lapseNotice(lifecycleState, isReadOnlyGrace)
  if (!notice) return null

  if (!notice.action) {
    return (
      <div className="border-destructive/20 bg-destructive/5 flex items-center border-b">
        <LapseMessage message={notice.message} error={error} />
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={() =>
        void (notice.action === 'resubscribe' ? resubscribe() : migrate())
      }
      aria-disabled={resolving}
      className="border-destructive/20 bg-destructive/5 hover:bg-destructive/8 flex w-full items-center border-b text-left transition-colors disabled:cursor-default"
    >
      <LapseMessage message={notice.message} error={error} />
      <div className="group flex shrink-0 items-center gap-1 px-3 py-1.5 transition-colors">
        <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
          {resolving ? 'Opening billing…' : notice.actionLabel}
        </span>
        <LuArrowRight className="text-foreground-alt group-hover:text-foreground size-3 shrink-0 transition-colors" />
      </div>
    </button>
  )
}
