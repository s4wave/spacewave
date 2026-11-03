import { useCallback, useState } from 'react'
import {
  LuCreditCard,
  LuPencil,
  LuRefreshCw,
  LuSave,
  LuX,
} from '@s4wave/web/images/icons.js'
import { cn } from '@s4wave/web/style/utils.js'
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

const stylexStyles2 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    minWidth: 0,
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule4: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule10: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule12: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'flex-start',
    justifyContent: 'center',
    overflowY: 'auto',
    paddingTop: `calc(${themeLayout['--spacing']} * 16)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule13: {
    width: '100%',
    maxWidth: 'var(--container-md)',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-sm)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
})

function statusBadgeColor(status?: BillingStatus): string {
  if (isStatusActive(status)) return 'bg-green-500/15 text-green-500'
  if (isStatusPastDue(status)) return 'bg-yellow-500/15 text-yellow-500'
  if (status === BillingStatus.BillingStatus_CANCELED)
    return 'bg-destructive/15 text-destructive'
  return 'bg-foreground/10 text-foreground-alt/70'
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
function BillingStatusRow({ billing }: { billing: BillingAccountInfo }) {
  const status = billing.status
  const intLabel = intervalLabel(billing.billingInterval)
  const isCancelScheduled = isStatusActive(status) && !!billing.cancelAt
  const renewalAt = billing.cancelAt || billing.currentPeriodEnd

  return (
    <>
      <div sx={stylexStyles2.rule7}>
        <span
          className={
            /* TODO(stylex): computed class helper */ cn(
              'micro-ten rounded-full px-2 py-0.5 font-semibold tracking-wider uppercase',
              statusBadgeColor(status),
            )
          }
        >
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
        <div sx={stylexStyles2.rule10}>
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
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-6 */ 'space-y-6'
            }
          >
            <BillingStatusRow billing={billing} />
            <UsageBars
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
                baId={baId}
                managedBillingAccount={managed.account}
                loading={managed.loading}
                onChanged={managed.reload}
              />
            )}
            <PlanControls
              status={billing.status}
              cancelAt={billing.cancelAt}
              showSelfService={billingState.selfServiceAllowed}
            />
            {baId && (
              <DeleteBillingAccountSection
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
