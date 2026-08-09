import { useCallback, useState } from 'react'
import { LuCreditCard, LuPlus } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { CheckoutStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { useCloudProviderConfig } from '../provider/spacewave/useSpacewaveAuth.js'
import { getCheckoutResultBaseUrl } from '../provider/spacewave/checkout-url.js'
import { BillingAccountsList } from './BillingAccountsList.js'
import { DetachAssignmentDialog } from './DetachAssignmentDialog.js'
import { useBillingAssignments } from './useBillingAssignments.js'
import { useManagedBillingAccounts } from './useManagedBillingAccounts.js'

// BillingAccountsPage coordinates billing-account list actions and navigation.
export function BillingAccountsPage() {
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl = getCheckoutResultBaseUrl(cloudProviderConfig)
  const { data, loading, error, store } = useManagedBillingAccounts()
  const assignments = useBillingAssignments()

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const handleCreate = useCallback(async () => {
    if (!store || !checkoutResultBaseUrl || creating) return
    setCreating(true)
    setCreateError(null)
    try {
      const baId = await store.create('Billing Account')
      const response = await store.createCheckoutSession({
        billingAccountId: baId,
        successUrl: checkoutResultBaseUrl + '/checkout/success',
        cancelUrl: checkoutResultBaseUrl + '/checkout/cancel',
      })
      if (response.status !== CheckoutStatus.CheckoutStatus_COMPLETED) {
        const url = response.checkoutUrl ?? ''
        if (url) window.open(url, '_blank', 'noopener')
      }
      navigateSession({ path: `billing/${baId}` })
    } catch (cause) {
      setCreateError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setCreating(false)
    }
  }, [checkoutResultBaseUrl, creating, navigateSession, store])

  const accounts = data?.accounts ?? []
  return (
    <div className="relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-8">
      <BackButton floating onClick={() => navigate({ path: '../' })}>
        Back
      </BackButton>
      <div className="w-full max-w-md px-4">
        <div className="mb-6 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <LuCreditCard className="text-foreground size-5" />
            <h1 className="text-foreground text-lg font-semibold tracking-tight">
              Billing Accounts
            </h1>
          </div>
          <button
            onClick={() => void handleCreate()}
            disabled={creating || !checkoutResultBaseUrl}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-brand flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            <LuPlus className="size-3.5" />
            <span>{creating ? 'Creating…' : 'New billing account'}</span>
          </button>
        </div>
        {[createError, assignments.assignError, assignments.detachError]
          .filter(Boolean)
          .map((message) => (
            <div
              key={message}
              className="border-destructive/20 bg-destructive/5 text-destructive mb-3 rounded-md border px-3 py-2 text-sm"
            >
              {message}
            </div>
          ))}
        {loading && !data && (
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
            icon={<LuCreditCard className="text-foreground-alt size-7" />}
            title="No billing accounts yet"
            description="A billing account holds your subscription. Create one, run checkout to activate it, then assign it to your personal account or to an organization you own."
            action={{
              label: creating
                ? 'Creating...'
                : 'Create your first BillingAccount',
              onClick: () => void handleCreate(),
            }}
            className="border-foreground/10 bg-foreground/5 rounded-md border"
          />
        )}
        {accounts.length > 0 && (
          <BillingAccountsList
            accounts={accounts}
            callerAccountId={assignments.callerAccountId}
            assignTargets={assignments.assignTargets}
            assigningBillingAccountId={assignments.assigningBillingAccountId}
            actionsDisabled={assignments.actionsDisabled}
            onOpen={(baId) => navigateSession({ path: `billing/${baId}` })}
            onAssign={(baId, target) => void assignments.assign(baId, target)}
            onDetach={assignments.requestDetach}
          />
        )}
      </div>
      <DetachAssignmentDialog
        target={assignments.detachTarget}
        busy={assignments.detaching}
        onCancel={assignments.cancelDetach}
        onConfirm={() => void assignments.confirmDetach()}
      />
    </div>
  )
}
