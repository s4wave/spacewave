import { useCallback, useMemo, useState } from 'react'
import { LuBuilding2, LuPlus, LuSettings, LuZap } from 'react-icons/lu'
import { RxPerson } from 'react-icons/rx'

import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  SessionContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import {
  BillingStatus,
  type ManagedBillingAccount,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  isStatusActive,
  lifecycleStateLabel,
  subscriptionStatusBadgeColor,
  subscriptionStatusLabel,
} from '@s4wave/app/billing/billing-utils.js'
import { PageFooter, PageWrapper } from './CloudConfirmationPage.js'
import { useBillingAccountCheckout } from './useBillingAccountCheckout.js'

interface BillingSetupTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

function getNoActiveBillingTargetOverride(): {
  ownerType: 'account' | 'organization'
  ownerId: string
} | null {
  const hash =
    window.location.hash.startsWith('#') ?
      window.location.hash.slice(1)
    : window.location.hash
  const query = hash.split('?')[1] ?? ''
  const params = new URLSearchParams(query)
  const ownerType = params.get('ownerType')
  const ownerId = params.get('ownerId')
  if (ownerType !== 'organization' || !ownerId) {
    return null
  }
  return { ownerType, ownerId }
}

function isAssignedToTarget(
  ba: ManagedBillingAccount,
  target: BillingSetupTarget,
): boolean {
  return (ba.assignees ?? []).some(
    (assignee) =>
      assignee.ownerType === target.ownerType &&
      assignee.ownerId === target.ownerId,
  )
}

// NoActiveBillingAccountPage lists the caller's managed billing accounts when
// none are currently active, and offers per-row activation plus a create-new
// CTA that routes through the standard checkout flow.
export function NoActiveBillingAccountPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigateSession = useSessionNavigate()
  const sessionIdx = useSessionIndex() || null
  const sessionMetadata = useSessionMetadata(sessionIdx)
  const { accountId: callerAccountId } = useSessionInfo(session)
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  const [activatingBaId, setActivatingBaId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const checkout = useBillingAccountCheckout({
    onCompleted: () => navigateSession({ path: 'setup' }),
  })

  const { data, loading, error } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session, reloadKey], // eslint-disable-line react-hooks/exhaustive-deps -- reloadKey triggers re-fetch
    ),
  )

  const accounts: ManagedBillingAccount[] = data?.accounts ?? []
  const hasAccounts = accounts.length > 0
  const targetOverride = getNoActiveBillingTargetOverride()
  const target = useMemo<BillingSetupTarget>(() => {
    if (targetOverride?.ownerType === 'organization') {
      const org = (orgListCtx?.organizations ?? []).find(
        (item) => item.id === targetOverride.ownerId,
      )
      return {
        ownerType: 'organization',
        ownerId: targetOverride.ownerId,
        label: org?.displayName || org?.id || 'Organization',
      }
    }
    return {
      ownerType: 'account',
      ownerId: callerAccountId,
      label:
        sessionMetadata?.displayName ||
        sessionMetadata?.cloudEntityId ||
        'Personal account',
    }
  }, [
    callerAccountId,
    orgListCtx?.organizations,
    sessionMetadata?.cloudEntityId,
    sessionMetadata?.displayName,
    targetOverride,
  ])
  const targetHasAssignedActiveBilling = useMemo(
    () =>
      accounts.some(
        (ba) =>
          isStatusActive(ba.subscriptionStatus) &&
          isAssignedToTarget(ba, target),
      ),
    [accounts, target],
  )
  const handleActivate = useCallback(
    async (ba: ManagedBillingAccount) => {
      const baId = ba.id ?? ''
      if (
        !session ||
        !target.ownerId ||
        !baId ||
        activatingBaId ||
        checkout.polling
      ) {
        return
      }
      setActivatingBaId(baId)
      setActionError(null)
      try {
        if (isStatusActive(ba.subscriptionStatus)) {
          if (!isAssignedToTarget(ba, target)) {
            await session.spacewave.assignBillingAccount(
              baId,
              target.ownerType,
              target.ownerId,
            )
          }
          navigateSession({ path: 'setup' })
          return
        }
        await checkout.startCheckout(baId)
      } catch (e) {
        setActionError(e instanceof Error ? e.message : String(e))
      } finally {
        setActivatingBaId(null)
      }
    },
    [activatingBaId, checkout, navigateSession, session, target],
  )

  const handleCreate = useCallback(async () => {
    if (!session || creating || checkout.polling) return
    setCreating(true)
    try {
      const sw = session.spacewave
      const baId = await sw.createBillingAccount('Billing Account')
      setReloadKey((k) => k + 1)
      await checkout.startCheckout(baId)
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    } finally {
      setCreating(false)
    }
  }, [checkout, creating, session])

  const handleManage = useCallback(
    (baId: string) => {
      navigateSession({ path: `billing/${baId}` })
    },
    [navigateSession],
  )

  const disableActions = !session || checkout.polling || !target.ownerId

  const title =
    hasAccounts ? 'Reactivate a billing account' : 'Create a billing account'
  const subtitle =
    hasAccounts ?
      'None of your billing accounts are currently active. Reactivate one below or create a new one to continue using Spacewave Cloud.'
    : 'Create a billing account to continue using Spacewave Cloud.'

  // Relative redirects resolve against the current URL, not the session
  // base. This component renders at /plan/no-active, so two levels up is
  // the session root. A bare "../setup" would land on /plan/setup which
  // has no route.
  if (targetHasAssignedActiveBilling) {
    return <Redirect to="../../setup" />
  }

  return (
    <PageWrapper>
      <div className="mt-4 flex w-full justify-start">
        <div className="border-foreground/10 bg-background-card/35 inline-flex items-center gap-3 rounded-xl border px-3 py-3 backdrop-blur-sm">
          <div className="bg-brand/10 text-brand flex h-10 w-10 items-center justify-center rounded-xl">
            {target.ownerType === 'organization' ?
              <LuBuilding2 className="h-5 w-5" />
            : <RxPerson className="h-5 w-5" />}
          </div>
          <div className="min-w-0">
            <div className="text-foreground-alt/60 text-[11px] font-medium tracking-[0.18em] uppercase">
              Setting billing for
            </div>
            <div className="text-foreground max-w-[16rem] truncate text-sm font-semibold tracking-tight">
              {target.label}
            </div>
          </div>
        </div>
      </div>

      <div className="flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <h1 className="mt-2 text-xl font-bold tracking-wide">{title}</h1>
        <p className="text-foreground-alt max-w-md text-center text-sm">
          {subtitle}
        </p>
      </div>

      {loading && (
        <div className="flex items-center justify-center">
          <LoadingInline
            label="Loading billing accounts"
            tone="muted"
            size="sm"
          />
        </div>
      )}

      {error && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-lg border px-3 py-2 text-sm backdrop-blur-sm">
          {error.message}
        </div>
      )}

      {(actionError || checkout.error) && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-lg border px-3 py-2 text-sm backdrop-blur-sm">
          {actionError || checkout.error}
        </div>
      )}

      {checkout.polling && (
        <div className="border-brand/20 bg-brand/5 rounded-lg border px-3 py-3 text-sm backdrop-blur-sm">
          <div className="flex items-center gap-2">
            <Spinner className="text-brand" />
            <span className="text-foreground">
              Activating subscription, this page will update when confirmation
              arrives.
            </span>
          </div>
          {checkout.showRetry && (
            <button
              onClick={checkout.continueCheckout}
              className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground mt-3 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
            >
              <LuZap className="h-3.5 w-3.5" />
              <span>Continue with Stripe</span>
            </button>
          )}
        </div>
      )}

      {hasAccounts && (
        <ul className="space-y-2">
          {accounts.map((ba) => {
            const baId = ba.id ?? ''
            const isBusy = activatingBaId === baId
            const isActive = isStatusActive(ba.subscriptionStatus)
            const activateLabel =
              isActive ? 'Use this billing account' : 'Activate'
            return (
              <li
                key={baId}
                className={cn(
                  'border-foreground/6 bg-background-card/30 rounded-lg border backdrop-blur-sm transition-all duration-150',
                  'hover:border-foreground/12 hover:bg-background-card/50',
                )}
              >
                <div className="flex flex-col gap-1 px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className="text-foreground text-sm font-medium select-none">
                      {ba.displayName || baId}
                    </span>
                    <span
                      className={cn(
                        'rounded-full border px-2 py-0.5 text-[0.55rem] font-semibold tracking-widest uppercase',
                        subscriptionStatusBadgeColor(ba.subscriptionStatus),
                      )}
                    >
                      {subscriptionStatusLabel(ba.subscriptionStatus)}
                    </span>
                  </div>
                  {ba.lifecycleState && (
                    <div className="text-foreground-alt/50 text-[0.6rem] select-none">
                      {lifecycleStateLabel(ba.lifecycleState)}
                    </div>
                  )}
                </div>
                <div className="border-foreground/6 flex items-center justify-end gap-2 border-t px-4 py-2">
                  <button
                    onClick={() => handleManage(baId)}
                    className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
                  >
                    <LuSettings className="h-3.5 w-3.5" />
                    <span className="select-none">Manage</span>
                  </button>
                  <button
                    onClick={() => void handleActivate(ba)}
                    disabled={isBusy || disableActions}
                    className={cn(
                      'flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-all duration-300 select-none',
                      'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
                      'disabled:cursor-not-allowed disabled:opacity-50',
                    )}
                  >
                    {isBusy ?
                      <Spinner size="sm" />
                    : <LuZap className="h-3.5 w-3.5" />}
                    <span>{isBusy ? 'Starting...' : activateLabel}</span>
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <div className="flex justify-center">
        <button
          onClick={() => void handleCreate()}
          disabled={creating || disableActions}
          className={cn(
            'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
            hasAccounts ?
              'border-foreground/20 bg-foreground/5 text-foreground hover:bg-foreground/10'
            : 'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {creating ?
            <Spinner />
          : <LuPlus className="h-4 w-4" />}
          <span>{creating ? 'Creating...' : 'Create new billing account'}</span>
        </button>
      </div>

      <PageFooter />
    </PageWrapper>
  )
}
