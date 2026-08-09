import { LuBuilding2, LuPlus, LuSettings, LuZap } from 'react-icons/lu'
import { RxPerson } from 'react-icons/rx'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { ManagedBillingAccount } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  isStatusActive,
  lifecycleStateLabel,
  subscriptionStatusBadgeColor,
  subscriptionStatusLabel,
} from '@s4wave/app/billing/billing-utils.js'

import { PageFooter } from './PageFooter.js'
import { PageWrapper } from './PageWrapper.js'

// NoActiveBillingAccountContent renders the billing-account selection surface.
export function NoActiveBillingAccountContent({
  target,
  title,
  subtitle,
  loading,
  errorMessage,
  actionError,
  checkoutError,
  checkoutPolling,
  checkoutShowRetry,
  accounts,
  activatingBaId,
  creating,
  disableActions,
  onActivate,
  onManage,
  onCreate,
  onContinueCheckout,
}: {
  target: { ownerType: 'account' | 'organization'; label: string }
  title: string
  subtitle: string
  loading: boolean
  errorMessage: string | null
  actionError: string | null
  checkoutError: string | null
  checkoutPolling: boolean
  checkoutShowRetry: boolean
  accounts: ManagedBillingAccount[]
  activatingBaId: string | null
  creating: boolean
  disableActions: boolean
  onActivate: (account: ManagedBillingAccount) => void
  onManage: (billingAccountID: string) => void
  onCreate: () => void
  onContinueCheckout: () => void
}) {
  const hasAccounts = accounts.length > 0

  return (
    <PageWrapper>
      <div className="mt-4 flex w-full justify-start">
        <div className="border-foreground/10 bg-background-card/35 inline-flex items-center gap-3 rounded-xl border p-3 backdrop-blur-sm">
          <div className="bg-brand/10 text-brand flex size-10 items-center justify-center rounded-xl">
            {target.ownerType === 'organization' ? (
              <LuBuilding2 className="size-5" />
            ) : (
              <RxPerson className="size-5" />
            )}
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
        <h1 className="mt-2 text-xl font-semibold tracking-wide">{title}</h1>
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

      {errorMessage && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-lg border px-3 py-2 text-sm backdrop-blur-sm">
          {errorMessage}
        </div>
      )}

      {(actionError || checkoutError) && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-lg border px-3 py-2 text-sm backdrop-blur-sm">
          {actionError || checkoutError}
        </div>
      )}

      {checkoutPolling && (
        <div className="border-brand/20 bg-brand/5 rounded-lg border p-3 text-sm backdrop-blur-sm">
          <div className="flex items-center gap-2">
            <Spinner className="text-brand" />
            <span className="text-foreground">
              Activating subscription, this page will update when confirmation
              arrives.
            </span>
          </div>
          {checkoutShowRetry && (
            <button
              onClick={onContinueCheckout}
              className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground mt-3 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
            >
              <LuZap className="size-3.5" />
              <span>Continue with Stripe</span>
            </button>
          )}
        </div>
      )}

      {hasAccounts && (
        <ul className="space-y-2">
          {accounts.map((account) => {
            const billingAccountID = account.id ?? ''
            const isBusy = activatingBaId === billingAccountID
            const isActive = isStatusActive(account.subscriptionStatus)
            const activateLabel = isActive
              ? 'Use this billing account'
              : 'Activate'
            return (
              <li
                key={billingAccountID}
                className={cn(
                  'border-foreground/6 bg-background-card/30 rounded-lg border backdrop-blur-sm transition-all duration-150',
                  'hover:border-foreground/12 hover:bg-background-card/50',
                )}
              >
                <div className="flex flex-col gap-1 px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className="text-foreground text-sm font-medium select-none">
                      {account.displayName || billingAccountID}
                    </span>
                    <span
                      className={cn(
                        'rounded-full border px-2 py-0.5 text-[0.55rem] font-semibold tracking-widest uppercase',
                        subscriptionStatusBadgeColor(
                          account.subscriptionStatus,
                        ),
                      )}
                    >
                      {subscriptionStatusLabel(account.subscriptionStatus)}
                    </span>
                  </div>
                  {account.lifecycleState && (
                    <div className="text-foreground-alt/50 text-[0.6rem] select-none">
                      {lifecycleStateLabel(account.lifecycleState)}
                    </div>
                  )}
                </div>
                <div className="border-foreground/6 flex items-center justify-end gap-2 border-t px-4 py-2">
                  <button
                    onClick={() => onManage(billingAccountID)}
                    className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
                  >
                    <LuSettings className="size-3.5" />
                    <span className="select-none">Manage</span>
                  </button>
                  <button
                    onClick={() => onActivate(account)}
                    disabled={isBusy || disableActions}
                    className={cn(
                      'flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-all duration-300 select-none',
                      'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
                      'disabled:cursor-not-allowed disabled:opacity-50',
                    )}
                  >
                    {isBusy ? (
                      <Spinner size="sm" />
                    ) : (
                      <LuZap className="size-3.5" />
                    )}
                    <span>{isBusy ? 'Starting…' : activateLabel}</span>
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <div className="flex justify-center">
        <button
          onClick={onCreate}
          disabled={creating || disableActions}
          className={cn(
            'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
            hasAccounts
              ? 'border-foreground/20 bg-foreground/5 text-foreground hover:bg-foreground/10'
              : 'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {creating ? <Spinner /> : <LuPlus className="size-4" />}
          <span>{creating ? 'Creating…' : 'Create new billing account'}</span>
        </button>
      </div>

      <PageFooter />
    </PageWrapper>
  )
}
