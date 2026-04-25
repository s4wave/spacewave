import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuCreditCard, LuPlus, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import {
  CheckoutStatus,
  type ManagedBillingAccount,
  type OrganizationInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DropdownTriggerButton } from '@s4wave/web/ui/DropdownTriggerButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ORG_ROLE_OWNER } from '../org/org-constants.js'
import { useCloudProviderConfig } from '../provider/spacewave/useSpacewaveAuth.js'
import { getCheckoutResultBaseUrl } from '../provider/spacewave/checkout-url.js'
import {
  DetachAssignmentDialog,
  type DetachAssignmentTarget,
} from './DetachAssignmentDialog.js'
import {
  lifecycleStateLabel,
  subscriptionStatusBadgeColor,
  subscriptionStatusLabel,
} from './billing-utils.js'

// AssignTarget is one option offered in the per-row "Assign to..." menu.
interface AssignTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

// BillingAccountsPage lists every BillingAccount the caller manages.
// Each row links into the detail view at /billing/:baId.
export function BillingAccountsPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const orgList = SpacewaveOrgListContext.useContext()
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl = getCheckoutResultBaseUrl(cloudProviderConfig)
  const { accountId: callerAccountId } = useSessionInfo(session)

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [assigningBaId, setAssigningBaId] = useState<string | null>(null)
  const [assignError, setAssignError] = useState<string | null>(null)
  const [detachTarget, setDetachTarget] =
    useState<DetachAssignmentTarget | null>(null)
  const [detaching, setDetaching] = useState(false)
  const [detachError, setDetachError] = useState<string | null>(null)

  const { data, loading, error } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session, reloadKey], // eslint-disable-line react-hooks/exhaustive-deps -- reloadKey triggers re-fetch after assign
    ),
  )

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleOpen = useCallback(
    (baId: string) => {
      navigateSession({ path: `billing/${baId}` })
    },
    [navigateSession],
  )

  const ownedOrgs: OrganizationInfo[] = useMemo(
    () => orgList.organizations.filter((o) => o.role === ORG_ROLE_OWNER),
    [orgList.organizations],
  )

  const assignTargets: AssignTarget[] = useMemo(() => {
    const list: AssignTarget[] = []
    if (callerAccountId) {
      list.push({
        ownerType: 'account',
        ownerId: callerAccountId,
        label: 'Personal account',
      })
    }
    for (const o of ownedOrgs) {
      if (!o.id) continue
      list.push({
        ownerType: 'organization',
        ownerId: o.id,
        label: o.displayName || o.id,
      })
    }
    return list
  }, [callerAccountId, ownedOrgs])

  const handleAssign = useCallback(
    async (baId: string, target: AssignTarget) => {
      if (!session || assigningBaId) return
      setAssigningBaId(baId)
      setAssignError(null)
      try {
        await session.spacewave.assignBillingAccount(
          baId,
          target.ownerType,
          target.ownerId,
        )
        setReloadKey((k) => k + 1)
      } catch (e) {
        setAssignError(e instanceof Error ? e.message : String(e))
      } finally {
        setAssigningBaId(null)
      }
    },
    [session, assigningBaId],
  )

  const handleDetachConfirm = useCallback(async () => {
    if (!session || !detachTarget || detaching) return
    setDetaching(true)
    setDetachError(null)
    try {
      await session.spacewave.detachBillingAccount(
        detachTarget.ownerType,
        detachTarget.ownerId,
      )
      setDetachTarget(null)
      setReloadKey((k) => k + 1)
    } catch (e) {
      setDetachError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetaching(false)
    }
  }, [session, detachTarget, detaching])

  const handleDetachCancel = useCallback(() => {
    if (detaching) return
    setDetachTarget(null)
    setDetachError(null)
  }, [detaching])

  const handleCreate = useCallback(async () => {
    if (!session || !checkoutResultBaseUrl || creating) return
    setCreating(true)
    setCreateError(null)
    try {
      const sw = session.spacewave
      const baId = await sw.createBillingAccount('Billing Account')
      const successUrl = checkoutResultBaseUrl + '/checkout/success'
      const cancelUrl = checkoutResultBaseUrl + '/checkout/cancel'
      const resp = await sw.createCheckoutSession({
        billingAccountId: baId,
        successUrl,
        cancelUrl,
      })
      if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
        navigateSession({ path: `billing/${baId}` })
        return
      }
      const url = resp.checkoutUrl ?? ''
      if (url) {
        window.open(url, '_blank')
      }
      navigateSession({ path: `billing/${baId}` })
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : String(e))
    } finally {
      setCreating(false)
    }
  }, [session, checkoutResultBaseUrl, creating, navigateSession])

  const accounts: ManagedBillingAccount[] = data?.accounts ?? []

  return (
    <div className="relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-8">
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div className="w-full max-w-md px-4">
        <div className="mb-6 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <LuCreditCard className="text-foreground h-5 w-5" />
            <h1 className="text-foreground text-lg font-semibold tracking-tight">
              Billing Accounts
            </h1>
          </div>
          <button
            onClick={() => void handleCreate()}
            disabled={creating || !checkoutResultBaseUrl}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-brand flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            <LuPlus className="h-3.5 w-3.5" />
            <span>{creating ? 'Creating...' : 'New billing account'}</span>
          </button>
        </div>
        {createError && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-sm">
            {createError}
          </div>
        )}
        {assignError && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-sm">
            {assignError}
          </div>
        )}
        {loading && (
          <div className="mx-auto w-full max-w-sm">
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading billing accounts',
                detail: 'Reading your billing accounts from the cloud.',
              }}
            />
          </div>
        )}
        {error && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-sm">
            {error.message}
          </div>
        )}
        {!loading && !error && accounts.length === 0 && (
          <EmptyState
            icon={<LuCreditCard className="text-foreground-alt h-7 w-7" />}
            title="No billing accounts yet"
            description="A billing account holds your subscription. Create one, run checkout to activate it, then assign it to your personal account or to an organization you own."
            action={{
              label:
                creating ? 'Creating...' : 'Create your first BillingAccount',
              onClick: () => void handleCreate(),
            }}
            className="border-foreground/10 bg-foreground/5 rounded-md border"
          />
        )}
        {detachError && (
          <div className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-sm">
            {detachError}
          </div>
        )}
        {accounts.length > 0 && (
          <ul className="space-y-2">
            {accounts.map((ba) => {
              const baId = ba.id ?? ''
              const assignees = ba.assignees ?? []
              const isBusy = assigningBaId === baId
              return (
                <li
                  key={baId}
                  className="border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 overflow-hidden rounded-md border transition-colors"
                >
                  <button
                    onClick={() => handleOpen(baId)}
                    className="flex w-full cursor-pointer flex-col gap-1 p-3 text-left"
                  >
                    <div className="flex items-center gap-2">
                      <span className="text-foreground text-sm font-medium">
                        {ba.displayName || baId}
                      </span>
                      <span
                        className={cn(
                          'rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wider uppercase',
                          subscriptionStatusBadgeColor(ba.subscriptionStatus),
                        )}
                      >
                        {subscriptionStatusLabel(ba.subscriptionStatus)}
                      </span>
                    </div>
                    <div className="text-foreground-alt/60 text-xs">
                      {[
                        lifecycleStateLabel(ba.lifecycleState),
                        ba.createdAt &&
                          `Created ${new Date(Number(ba.createdAt)).toLocaleDateString()}`,
                      ]
                        .filter(Boolean)
                        .join(' \u00b7 ')}
                    </div>
                  </button>
                  <div className="border-foreground/5 flex items-center justify-between gap-2 border-t px-3 py-2">
                    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
                      {assignees.length === 0 && (
                        <span className="text-foreground-alt/60 text-[11px]">
                          Unassigned
                        </span>
                      )}
                      {assignees.map((a) => {
                        const isPersonal =
                          a.ownerType === 'account' &&
                          a.ownerId === callerAccountId
                        const label =
                          isPersonal ? 'Personal' : (
                            a.displayName || a.ownerId || ''
                          )
                        return (
                          <span
                            key={`${a.ownerType}:${a.ownerId}`}
                            className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px]"
                          >
                            <span>{label}</span>
                            <button
                              onClick={() =>
                                setDetachTarget({
                                  ownerType: a.ownerType as
                                    | 'account'
                                    | 'organization',
                                  ownerId: a.ownerId ?? '',
                                  label,
                                })
                              }
                              aria-label={`Detach ${label}`}
                              className="hover:text-destructive cursor-pointer transition-colors"
                            >
                              <LuX className="h-3 w-3" />
                            </button>
                          </span>
                        )
                      })}
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        asChild
                        disabled={
                          !session || isBusy || assignTargets.length === 0
                        }
                      >
                        <DropdownTriggerButton triggerStyle="ghost">
                          {isBusy ? 'Assigning...' : 'Assign to...'}
                        </DropdownTriggerButton>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuLabel>Assign this BA to</DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        {assignTargets.map((t) => {
                          const isSelected = assignees.some(
                            (a) =>
                              a.ownerType === t.ownerType &&
                              a.ownerId === t.ownerId,
                          )
                          return (
                            <DropdownMenuItem
                              key={`${t.ownerType}:${t.ownerId}`}
                              onSelect={() => void handleAssign(baId, t)}
                            >
                              <LuCheck
                                className={cn(
                                  'h-3 w-3',
                                  isSelected ? 'text-brand' : (
                                    'text-transparent'
                                  ),
                                )}
                              />
                              <span>{t.label}</span>
                            </DropdownMenuItem>
                          )
                        })}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      <DetachAssignmentDialog
        target={detachTarget}
        busy={detaching}
        onCancel={handleDetachCancel}
        onConfirm={() => void handleDetachConfirm()}
      />
    </div>
  )
}
