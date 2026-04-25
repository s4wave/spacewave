import { useCallback, useEffect, useState } from 'react'
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
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { UsageBars } from './UsageBars.js'
import { PlanControls } from './PlanControls.js'
import { StripePortalLink } from './StripePortalLink.js'
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
  const status = billing?.status
  const interval = billing?.billingInterval
  const cancelAt = billing?.cancelAt
  const intLabel = intervalLabel(interval)
  const isCancelScheduled = isStatusActive(status) && !!cancelAt
  const renewalAt = cancelAt || billing?.currentPeriodEnd

  const [renaming, setRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const [renameSaving, setRenameSaving] = useState(false)
  const [renameError, setRenameError] = useState<string | null>(null)
  const [refreshingUsage, setRefreshingUsage] = useState(false)
  const [refreshError, setRefreshError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  const { data: managedData } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session, reloadKey],
    ),
  )

  const managedBillingAccount =
    (managedData?.accounts ?? []).find((row) => row.id === baId) ?? null
  const managedBillingLoading = !!session && managedData == null
  const assigneeCount = managedBillingAccount?.assignees?.length ?? 0
  const deleteDisabledReason =
    managedBillingLoading ? 'Loading billing account assignments...'
    : !managedBillingAccount ? 'Only the billing account creator can delete it.'
    : null

  useEffect(() => {
    if (!renaming) setRenameValue(displayName)
  }, [displayName, renaming])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleRenameStart = useCallback(() => {
    setRenameValue(displayName)
    setRenameError(null)
    setRenaming(true)
  }, [displayName])

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

  const handleRefreshUsage = useCallback(async () => {
    if (!session || refreshingUsage) return
    setRefreshError(null)
    setRefreshingUsage(true)
    try {
      await session.spacewave.refreshBillingState(baId || undefined)
    } catch (e) {
      setRefreshError(e instanceof Error ? e.message : String(e))
    } finally {
      setRefreshingUsage(false)
    }
  }, [session, baId, refreshingUsage])

  const title = displayName || 'Billing'

  return (
    <div className="relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-8">
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div className="w-full max-w-md px-4">
        <div className="mb-6 flex items-center gap-2">
          <LuCreditCard className="text-foreground h-5 w-5 shrink-0" />
          {renaming ?
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <input
                type="text"
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void handleRenameSave()
                  if (e.key === 'Escape') handleRenameCancel()
                }}
                autoFocus
                className={cn(
                  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 min-w-0 flex-1 rounded-md border px-2 py-1 text-sm transition-colors outline-none',
                  'focus:border-brand/50',
                )}
                placeholder="Billing account name"
                aria-label="Billing account name"
              />
              <DashboardButton
                icon={<LuSave className="h-3 w-3" />}
                onClick={() => void handleRenameSave()}
                disabled={
                  renameSaving ||
                  !renameValue.trim() ||
                  renameValue.trim() === displayName
                }
              >
                {renameSaving ? 'Saving...' : 'Save'}
              </DashboardButton>
              <DashboardButton
                icon={<LuX className="h-3 w-3" />}
                onClick={handleRenameCancel}
                disabled={renameSaving}
              >
                Cancel
              </DashboardButton>
            </div>
          : <div className="flex min-w-0 flex-1 items-center justify-between gap-2">
              <h1 className="text-foreground truncate text-lg font-semibold tracking-tight">
                {title}
              </h1>
              {billing && baId && (
                <DashboardButton
                  icon={<LuPencil className="h-3 w-3" />}
                  onClick={handleRenameStart}
                >
                  Edit
                </DashboardButton>
              )}
            </div>
          }
        </div>
        {renameError && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-xs">
            {renameError}
          </div>
        )}
        {refreshError && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-xs">
            {refreshError}
          </div>
        )}
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
            <div className="flex items-center gap-3">
              <span
                className={cn(
                  'rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wider uppercase',
                  statusBadgeColor(status),
                )}
              >
                {statusLabel(status)}
              </span>
              {intLabel && (
                <span className="text-foreground-alt/50 text-xs">
                  {intLabel}
                </span>
              )}
              {renewalAt && (
                <span className="text-foreground-alt/40 text-xs">
                  {isCancelScheduled ? 'Ends' : 'Renews'}{' '}
                  {new Date(Number(renewalAt)).toLocaleDateString()}
                </span>
              )}
            </div>
            {isCancelScheduled && renewalAt && (
              <div className="border-destructive/20 bg-destructive/5 text-foreground-alt rounded-md border px-3 py-2 text-xs leading-relaxed">
                Your subscription is set to end on{' '}
                <span className="text-foreground font-medium">
                  {new Date(Number(renewalAt)).toLocaleDateString()}
                </span>
                . You keep full access until then, and your cloud data stays
                read-only for 30 days afterward so you can export it.
              </div>
            )}
            <UsageBars
              actions={
                <DashboardButton
                  icon={
                    <LuRefreshCw
                      className={cn(
                        'h-3 w-3',
                        refreshingUsage && 'animate-spin',
                      )}
                    />
                  }
                  onClick={() => void handleRefreshUsage()}
                  disabled={!session || refreshingUsage}
                >
                  {refreshingUsage ? 'Refreshing...' : 'Refresh'}
                </DashboardButton>
              }
            />
            {baId && (
              <BillingAssignmentsSection
                baId={baId}
                managedBillingAccount={managedBillingAccount}
                loading={managedBillingLoading}
                onChanged={() => setReloadKey((k) => k + 1)}
              />
            )}
            <PlanControls
              status={status}
              cancelAt={cancelAt}
              showSelfService={billingState.selfServiceAllowed}
            />
            <StripePortalLink />
            {baId && (
              <DeleteBillingAccountSection
                billingAccountId={baId}
                displayName={title}
                status={status}
                assigneeCount={assigneeCount}
                disabledReasonOverride={deleteDisabledReason}
                onDeleted={() => navigate({ path: '../' })}
              />
            )}
          </div>
        )}
      </div>
    </div>
  )
}
