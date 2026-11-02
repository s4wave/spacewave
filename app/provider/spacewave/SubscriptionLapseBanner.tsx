import { useCallback, useState } from 'react'
import { LuTriangleAlert, LuArrowRight } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

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
      <LuTriangleAlert sx={stylexStyles.rule1} />
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
        <LuTriangleAlert sx={stylexStyles.rule2} />
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
      <div sx={[stylexStyles2.rule1, groupMarker2]}>
        <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
          {resolving ? 'Opening billing…' : notice.actionLabel}
        </span>
        <LuArrowRight sx={stylexStyles.rule3} />
      </div>
    </button>
  )
}
