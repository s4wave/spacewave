import { keyframeNames } from '../../web/style/tokens.stylex.js'
import { useCallback, useState } from 'react'
import {
  LuCreditCard,
  LuPencil,
  LuRefreshCw,
  LuSave,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import {
  BillingStatus,
  type BillingAccountInfo,
  type ManagedBillingAccount,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { UsageBars } from './UsageBars.js'
import { PlanControls } from './PlanControls.js'
import { BillingAssignmentsSection } from './BillingAssignmentsSection.js'
import { DeleteBillingAccountSection } from './DeleteBillingAccountSection.js'
import {
  statusLabel,
  intervalLabel,
  isStatusActive,
  isStatusPastDue,
} from './billing-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 15%, transparent)`,
    color: themeColors['--color-green-500'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 15%, transparent)`,
    color: themeColors['--color-yellow-500'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule3: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minWidth: 0,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule4: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    minWidth: 0,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    '--tw-leading': 'var(--leading-relaxed)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule12: {
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: '100%',
    overflowY: 'auto',
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 16)`,
    width: '100%',
  },
  rule13: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule14: {
    marginInline: 'auto',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['spin'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'linear',
  },
})

function statusBadgeStyles(status?: BillingStatus): stylex.StyleXStyles {
  if (isStatusActive(status)) return stylexStyles3.rule1
  if (isStatusPastDue(status)) return stylexStyles3.rule2
  if (status === BillingStatus.BillingStatus_CANCELED)
    return stylexStyles3.rule3
  return stylexStyles3.rule4
}

function formatBillingDate(timestampMs: number | string | bigint): string {
  return new Date(Number(timestampMs)).toLocaleDateString()
}

interface ManagedBilling {
  account: ManagedBillingAccount | null
  loading: boolean
  deleteDisabledReason: string | null
  reload: () => void
}

/** useManagedBilling loads the billing account as seen by the session's manager list. */
function useManagedBilling(
  session: Session | null | undefined,
  baId: string,
): ManagedBilling {
  const [reloadKey, setReloadKey] = useState(0)
  const { data: managedData } = usePromise(
    useCallback(
      (signal: AbortSignal) => {
        void reloadKey
        return (
          session?.spacewave.listManagedBillingAccounts(signal) ??
          Promise.resolve(null)
        )
      },
      [session, reloadKey],
    ),
  )
  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  const account =
    (managedData?.accounts ?? []).find((row) => row.id === baId) ?? null
  const loading = !!session && managedData == null
  let deleteDisabledReason: string | null = null
  if (loading) {
    deleteDisabledReason = 'Loading billing account assignments...'
  } else if (!account) {
    deleteDisabledReason = 'Only the billing account creator can delete it.'
  }
  return { account, loading, deleteDisabledReason, reload }
}

interface UsageRefresh {
  refreshing: boolean
  error: string | null
  refresh: () => Promise<void>
}

/** useUsageRefresh refreshes the billing state of the account and reports its failure. */
function useUsageRefresh(
  session: Session | null | undefined,
  baId: string,
): UsageRefresh {
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!session || refreshing) return
    setError(null)
    setRefreshing(true)
    try {
      await session.spacewave.refreshBillingState(baId || undefined)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setRefreshing(false)
    }
  }, [session, baId, refreshing])

  return { refreshing, error, refresh }
}

interface BillingTitleProps {
  session: Session | null | undefined
  baId: string
  displayName: string
  title: string
  canRename: boolean
}

/** BillingTitle shows the account name with an inline rename editor and its error. */
function BillingTitle({
  session,
  baId,
  displayName,
  title,
  canRename,
}: BillingTitleProps) {
  const [renaming, setRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const [renameSaving, setRenameSaving] = useState(false)
  const [renameError, setRenameError] = useState<string | null>(null)

  const handleRenameStart = useCallback(() => {
    setRenameValue(displayName)
    setRenameError(null)
    setRenaming(true)
  }, [displayName])

  const handleRenameInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleRenameCancel = useCallback(() => {
    setRenaming(false)
    setRenameError(null)
  }, [])

  const handleRenameSave = useCallback(async () => {
    if (!session || !baId || renameSaving) return
    const next = renameValue.trim()
    if (!next || next === displayName) {
      setRenaming(false)
      return
    }
    setRenameSaving(true)
    setRenameError(null)
    try {
      await session.spacewave.renameBillingAccount(baId, next)
      setRenaming(false)
    } catch (e) {
      setRenameError(e instanceof Error ? e.message : String(e))
    } finally {
      setRenameSaving(false)
    }
  }, [session, baId, renameValue, displayName, renameSaving])

  return (
    <>
      <div sx={stylexStyles2.rule1}>
        <LuCreditCard sx={stylexStyles.rule1} />
        {renaming ? (
          <div sx={stylexStyles2.rule2}>
            <input
              ref={handleRenameInputRef}
              type="text"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return
                if (e.key === 'Enter') void handleRenameSave()
                if (e.key === 'Escape') handleRenameCancel()
              }}
              sx={[stylexStyles2.rule3]}
              placeholder="Billing account name"
              aria-label="Billing account name"
            />
            <DashboardButton
              icon={<LuSave sx={stylexStyles.rule2} />}
              onClick={() => void handleRenameSave()}
              disabled={
                renameSaving ||
                !renameValue.trim() ||
                renameValue.trim() === displayName
              }
            >
              {renameSaving ? 'Saving…' : 'Save'}
            </DashboardButton>
            <DashboardButton
              icon={<LuX sx={stylexStyles.rule2} />}
              onClick={handleRenameCancel}
              disabled={renameSaving}
            >
              Cancel
            </DashboardButton>
          </div>
        ) : (
          <div sx={stylexStyles2.rule4}>
            <h1 sx={stylexStyles2.rule5}>{title}</h1>
            {canRename && (
              <DashboardButton
                icon={<LuPencil sx={stylexStyles.rule2} />}
                onClick={handleRenameStart}
              >
                Edit
              </DashboardButton>
            )}
          </div>
        )}
      </div>
      <InlineError message={renameError} />
    </>
  )
}

/** InlineError renders a destructive notice, or nothing without a message. */
function InlineError({ message }: { message: string | null }) {
  if (!message) return null
  return <div sx={stylexStyles2.rule6}>{message}</div>
}

/** BillingStatusRow shows the plan status, interval, and renewal or end date. */
function BillingStatusRow({
  sx,
  billing,
}: { billing: BillingAccountInfo } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const status = billing.status
  const intLabel = intervalLabel(billing.billingInterval)
  const isCancelScheduled = isStatusActive(status) && !!billing.cancelAt
  const renewalAt = billing.cancelAt || billing.currentPeriodEnd

  return (
    <>
      <div sx={[stylexStyles2.rule7, sx]}>
        <span sx={[[stylexStyles4.rule1], [statusBadgeStyles(status)]]}>
          {statusLabel(status)}
        </span>
        {intLabel && <span sx={stylexStyles2.rule8}>{intLabel}</span>}
        {renewalAt && (
          <span suppressHydrationWarning sx={stylexStyles2.rule9}>
            {isCancelScheduled ? 'Ends' : 'Renews'}{' '}
            {formatBillingDate(renewalAt)}
          </span>
        )}
      </div>
      {isCancelScheduled && renewalAt && (
        <div sx={[stylexStyles2.rule10, sx]}>
          Your subscription is set to end on{' '}
          <span suppressHydrationWarning sx={stylexStyles2.rule11}>
            {formatBillingDate(renewalAt)}
          </span>
          . You keep full access until then, and your cloud data stays read-only
          for 30 days afterward so you can export it.
        </div>
      )}
    </>
  )
}

// BillingPage displays billing state and usage for a billing account.
// Used for both personal and org billing.
export function BillingPage() {
  const billingState = useBillingStateContext()
  const navigate = useNavigate()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)

  const billing = billingState.response?.billingAccount
  const baId = billingState.billingAccountId ?? ''
  const displayName = billing?.displayName ?? ''
  const title = displayName || 'Billing'
  const managed = useManagedBilling(session, baId)
  const usage = useUsageRefresh(session, baId)

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  return (
    <div sx={stylexStyles2.rule12}>
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div sx={stylexStyles2.rule13}>
        <BillingTitle
          session={session}
          baId={baId}
          displayName={displayName}
          title={title}
          canRename={!!billing && !!baId}
        />
        <InlineError message={usage.error} />
        {billingState.loading && !billing && (
          <div sx={stylexStyles2.rule14}>
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading billing account',
                detail: 'Reading account status, usage, and assignments.',
              }}
            />
          </div>
        )}
        {billing && (
          <div sx={false}>
            <BillingStatusRow sx={nativeChildSpacing.s6} billing={billing} />
            <UsageBars
              sx={nativeChildSpacing.s6}
              actions={
                <DashboardButton
                  icon={
                    <LuRefreshCw
                      sx={[
                        stylexStyles.rule2,
                        !!usage.refreshing && stylexStyles.rule3,
                      ]}
                    />
                  }
                  onClick={() => void usage.refresh()}
                  disabled={!session || usage.refreshing}
                >
                  {usage.refreshing ? 'Refreshing…' : 'Refresh'}
                </DashboardButton>
              }
            />
            {baId && (
              <BillingAssignmentsSection
                sx={nativeChildSpacing.s6}
                baId={baId}
                managedBillingAccount={managed.account}
                loading={managed.loading}
                onChanged={managed.reload}
              />
            )}
            <PlanControls
              sx={nativeChildSpacing.s6}
              status={billing.status}
              cancelAt={billing.cancelAt}
              showSelfService={billingState.selfServiceAllowed}
            />
            {baId && (
              <DeleteBillingAccountSection
                sx={nativeChildSpacing.s6}
                billingAccountId={baId}
                displayName={title}
                status={billing.status}
                assigneeCount={managed.account?.assignees?.length ?? 0}
                disabledReasonOverride={managed.deleteDisabledReason}
                onDeleted={() => navigate({ path: '../' })}
              />
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
  },
})
