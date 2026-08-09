import { useCallback, useMemo, useState } from 'react'

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
import { usePath } from '@s4wave/web/router/router.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import type { ManagedBillingAccount } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { isStatusActive } from '@s4wave/app/billing/billing-utils.js'
import { useBillingAccountCheckout } from './useBillingAccountCheckout.js'
import { NoActiveBillingAccountContent } from './NoActiveBillingAccountContent.js'

interface BillingSetupTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

function getNoActiveBillingTargetOverride(path: string): {
  ownerType: 'account' | 'organization'
  ownerId: string
} | null {
  const query = path.split('?')[1] ?? ''
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
  const path = usePath()
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

  const accounts = useMemo<ManagedBillingAccount[]>(
    () => data?.accounts ?? [],
    [data?.accounts],
  )
  const hasAccounts = accounts.length > 0
  const targetOverride = useMemo(
    () => getNoActiveBillingTargetOverride(path),
    [path],
  )
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

  const title = hasAccounts
    ? 'Reactivate a billing account'
    : 'Create a billing account'
  const subtitle = hasAccounts
    ? 'None of your billing accounts are currently active. Reactivate one below or create a new one to continue using Spacewave Cloud.'
    : 'Create a billing account to continue using Spacewave Cloud.'

  // Relative redirects resolve against the current URL, not the session
  // base. This component renders at /plan/no-active, so two levels up is
  // the session root. A bare "../setup" would land on /plan/setup which
  // has no route.
  if (targetHasAssignedActiveBilling) {
    return <Redirect to="../../setup" />
  }

  return (
    <NoActiveBillingAccountContent
      target={target}
      title={title}
      subtitle={subtitle}
      loading={loading}
      errorMessage={error?.message ?? null}
      actionError={actionError}
      checkoutError={checkout.error}
      checkoutPolling={checkout.polling}
      checkoutShowRetry={checkout.showRetry}
      accounts={accounts}
      activatingBaId={activatingBaId}
      creating={creating}
      disableActions={disableActions}
      onActivate={(account) => void handleActivate(account)}
      onManage={handleManage}
      onCreate={() => void handleCreate()}
      onContinueCheckout={checkout.continueCheckout}
    />
  )
}
