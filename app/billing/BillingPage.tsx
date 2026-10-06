import { useCallback, useState } from 'react'
import {
  LuCreditCard,
  LuPencil,
  LuRefreshCw,
  LuSave,
  LuX,
} from 'react-icons/lu'
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
      <div className="mb-6 flex items-center gap-2">
        <LuCreditCard className="text-foreground size-5 shrink-0" />
        {renaming ? (
          <div className="flex min-w-0 flex-1 items-center gap-2">
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
              className={cn(
                'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 min-w-0 flex-1 rounded-md border px-2 py-1 text-sm transition-colors outline-none',
                'focus:border-brand/50',
              )}
              placeholder="Billing account name"
              aria-label="Billing account name"
            />
            <DashboardButton
              icon={<LuSave className="size-3" />}
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
              icon={<LuX className="size-3" />}
              onClick={handleRenameCancel}
              disabled={renameSaving}
            >
              Cancel
            </DashboardButton>
          </div>
        ) : (
          <div className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <h1 className="text-foreground truncate text-lg font-semibold tracking-tight">
              {title}
            </h1>
            {canRename && (
              <DashboardButton
                icon={<LuPencil className="size-3" />}
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
  return (
    <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-xs">
      {message}
    </div>
  )
}

/** BillingStatusRow shows the plan status, interval, and renewal or end date. */
function BillingStatusRow({ billing }: { billing: BillingAccountInfo }) {
  const status = billing.status
  const intLabel = intervalLabel(billing.billingInterval)
  const isCancelScheduled = isStatusActive(status) && !!billing.cancelAt
  const renewalAt = billing.cancelAt || billing.currentPeriodEnd

  return (
    <>
      <div className="flex items-center gap-3">
        <span
          className={cn(
            'micro-ten rounded-full px-2 py-0.5 font-semibold tracking-wider uppercase',
            statusBadgeColor(status),
          )}
        >
          {statusLabel(status)}
        </span>
        {intLabel && (
          <span className="text-foreground-alt/50 text-xs">{intLabel}</span>
        )}
        {renewalAt && (
          <span
            suppressHydrationWarning
            className="text-foreground-alt/40 text-xs"
          >
            {isCancelScheduled ? 'Ends' : 'Renews'}{' '}
            {formatBillingDate(renewalAt)}
          </span>
        )}
      </div>
      {isCancelScheduled && renewalAt && (
        <div className="border-destructive/20 bg-destructive/5 text-foreground-alt rounded-md border px-3 py-2 text-xs leading-relaxed">
          Your subscription is set to end on{' '}
          <span
            suppressHydrationWarning
            className="text-foreground font-medium"
          >
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
    <div className="relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-8">
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div className="w-full max-w-md px-4">
        <BillingTitle
          session={session}
          baId={baId}
          displayName={displayName}
          title={title}
          canRename={!!billing && !!baId}
        />
        <InlineError message={usage.error} />
        {billingState.loading && !billing && (
          <div className="mx-auto w-full max-w-sm">
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
          <div className="space-y-6">
            <BillingStatusRow billing={billing} />
            <UsageBars
              actions={
                <DashboardButton
                  icon={
                    <LuRefreshCw
                      className={cn(
                        'size-3',
                        usage.refreshing && 'animate-spin',
                      )}
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
