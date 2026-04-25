import { useCallback, useState } from 'react'
import {
  LuArrowLeft,
  LuClock3,
  LuCalendarX,
  LuDownload,
  LuRefreshCw,
  LuShield,
  LuTrash2,
  LuTriangleAlert,
} from 'react-icons/lu'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  FaqAccordion,
  PageFooter,
  PageWrapper,
} from '@s4wave/app/provider/spacewave/CloudConfirmationPage.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'

const CANCEL_FAQ = [
  {
    question: 'Can I change my mind later?',
    answer:
      'Yes. Until the plan actually ends, you can keep the subscription active again from the billing page.',
  },
  {
    question: 'What happens to my data after cancellation?',
    answer:
      'Nothing disappears right away. You keep full access until the end date. After that, cloud data becomes read-only for 30 days so you can export it.',
  },
  {
    question: 'How do prorated refunds work?',
    answer:
      'Prorated refunds are only available if you delete your account, not with standard cancellation.',
  },
  {
    question: 'Can I come back later?',
    answer:
      'Yes. You can start a new Cloud subscription again later if you want to return.',
  },
]

// BillingCancelPage explains cancellation outcomes before confirming.
export function BillingCancelPage() {
  const navigate = useNavigate()
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const checkout = useBillingAccountCheckout({
    onCompleted: () => navigate({ path: '../' }),
  })
  const [action, setAction] = useState<'idle' | 'canceling' | 'reactivating'>(
    'idle',
  )
  const [error, setError] = useState<string | null>(null)

  const billing = billingState.response?.billingAccount
  const lifecycleState = billing?.lifecycleState
  const cancelAt = billing?.cancelAt
  const endAt = cancelAt || billing?.currentPeriodEnd
  const isCancelScheduled =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END
  const isGrace =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY
  const canScheduleCancel =
    lifecycleState === AccountLifecycleState.AccountLifecycleState_ACTIVE ||
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END
  const endLabel =
    endAt ?
      new Date(Number(endAt)).toLocaleDateString(undefined, {
        month: 'long',
        day: 'numeric',
        year: 'numeric',
      })
    : null
  const title =
    isCancelScheduled ?
      endLabel ? `Your plan will already cancel on ${endLabel}`
      : 'Your plan is already set to cancel'
    : isGrace ? 'Your plan is in the 30-day export window'
    : 'Cancel your Spacewave Cloud plan?'
  const subtitle =
    isCancelScheduled ?
      'Nothing else needs to happen. You still have full access until then. If you changed your mind, you can keep the plan active.'
    : isGrace ?
      'Your subscription has already ended. Cloud data is read-only for 30 days so you can export what you need or start a new subscription.'
    : 'This keeps your plan active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export it.'

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleCancel = useCallback(async () => {
    if (!session || action !== 'idle') return
    setAction('canceling')
    setError(null)
    try {
      await session.spacewave.cancelSubscription(billingState.billingAccountId)
      navigate({ path: '../' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Cancel failed')
      setAction('idle')
    }
  }, [session, action, navigate, billingState.billingAccountId])

  const handleKeep = useCallback(async () => {
    const baId = billingState.billingAccountId
    if (!session || !baId || action !== 'idle') return
    setAction('reactivating')
    setError(null)
    try {
      const resp = await session.spacewave.reactivateSubscription(baId)
      if (resp.needsCheckout) {
        await checkout.startCheckout(baId)
        return
      }
      navigate({ path: '../' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reactivate failed')
      setAction('idle')
    }
  }, [session, action, navigate, billingState.billingAccountId, checkout])

  return (
    <PageWrapper
      backButton={
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-brand flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to billing
        </button>
      }
    >
      <div className="flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <div className="border-brand/25 bg-brand/8 text-brand mt-2 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-[11px] font-medium tracking-wide uppercase">
          <LuCalendarX className="h-3.5 w-3.5" />
          {isCancelScheduled ?
            'Cancellation scheduled'
          : isGrace ?
            'Read-only export window'
          : 'End-of-period cancellation'}
        </div>
        <h1 className="mt-2 text-center text-xl font-bold tracking-wide">
          {title}
        </h1>
        <p className="text-foreground-alt max-w-xl text-center text-sm leading-relaxed">
          {subtitle}
        </p>
      </div>

      <div className="border-brand/20 bg-background-card/55 overflow-hidden rounded-xl border p-8 backdrop-blur-sm">
        <div className="mb-6 flex items-start gap-3">
          <div className="bg-brand/10 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <LuCalendarX className="text-brand h-5 w-5" />
          </div>
          <div className="space-y-2">
            <h2 className="text-foreground text-lg font-semibold">
              {isCancelScheduled || isGrace ?
                'What happens next'
              : 'If you cancel now'}
            </h2>
            <p className="text-foreground-alt text-sm leading-relaxed">
              {isGrace ?
                'Your plan has already ended. Cloud data is read-only for 30 days so you can export or start a new subscription.'
              : endLabel ?
                <>
                  You keep full access through{' '}
                  <span className="text-foreground font-medium">
                    {endLabel}
                  </span>
                  . After that, your cloud data stays read-only for 30 days so
                  you can export what you need.
                </>
              : 'You keep full access through the end of the current billing period. After that, your cloud data stays read-only for 30 days so you can export what you need.'
              }
            </p>
          </div>
        </div>

        <div className="mt-6 grid gap-3 sm:grid-cols-3">
          <div className="border-foreground/10 bg-background/45 rounded-lg border p-4">
            <div className="mb-2 flex items-center gap-2">
              <div className="bg-brand/10 flex h-8 w-8 items-center justify-center rounded-md">
                <LuClock3 className="text-brand h-4 w-4" />
              </div>
              <h3 className="text-foreground text-sm font-semibold">
                {isGrace ? 'Read-only access' : 'Full access'}
              </h3>
            </div>
            <p className="text-foreground-alt text-sm leading-relaxed text-balance">
              {isGrace ?
                'The subscription has already ended. You can still export existing cloud data during the remaining 30-day window.'
              : endLabel ?
                <>Everything keeps working normally until {endLabel}.</>
              : 'Everything keeps working normally until the current billing period ends.'
              }
            </p>
          </div>
          <div className="border-foreground/10 bg-background/45 rounded-lg border p-4">
            <div className="mb-2 flex items-center gap-2">
              <div className="bg-brand/10 flex h-8 w-8 items-center justify-center rounded-md">
                <LuDownload className="text-brand h-4 w-4" />
              </div>
              <h3 className="text-foreground text-sm font-semibold">
                30-day export window
              </h3>
            </div>
            <p className="text-foreground-alt text-sm leading-relaxed text-balance">
              Cloud data stays read-only for 30 days after the plan ends so you
              can export it safely or re-subscribe.
            </p>
          </div>
          <div className="border-foreground/10 bg-background/45 rounded-lg border p-4">
            <div className="mb-2 flex items-center gap-2">
              <div className="bg-brand/10 flex h-8 w-8 items-center justify-center rounded-md">
                <LuShield className="text-brand h-4 w-4" />
              </div>
              <h3 className="text-foreground text-sm font-semibold">
                Easy to undo
              </h3>
            </div>
            <p className="text-foreground-alt text-sm leading-relaxed text-balance">
              {isGrace ?
                'Start a new subscription whenever you want to restore read and write access.'
              : isCancelScheduled ?
                'If you changed your mind, keep the subscription active with one click.'
              : 'If you change your mind later, you can keep the subscription active before it ends.'
              }
            </p>
          </div>
        </div>

        <div className="border-foreground/10 bg-background/45 mt-6 rounded-lg border p-4">
          <div className="flex items-start gap-3">
            <div className="bg-destructive/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
              <LuTriangleAlert className="text-destructive h-4 w-4" />
            </div>
            <div className="space-y-1">
              <h3 className="text-foreground text-sm font-semibold">
                Need account deletion instead?
              </h3>
              <p className="text-foreground-alt text-sm leading-relaxed">
                Deleting your account requires email verification and works
                differently from standard end-of-period cancellation.
              </p>
              <div className="text-foreground-alt/80 flex items-center gap-2 pt-1 text-sm">
                <LuTrash2 className="h-3.5 w-3.5" />
                Go to account settings and choose{' '}
                <span className="text-foreground font-medium">
                  Delete account
                </span>{' '}
                if that is the path you want.
              </div>
            </div>
          </div>
        </div>

        <div className="mt-8 flex flex-col gap-3 sm:flex-row">
          {isCancelScheduled ?
            <button
              onClick={() => void handleKeep()}
              disabled={action !== 'idle' || checkout.polling}
              className={cn(
                'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300',
                'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              <LuRefreshCw
                className={cn(
                  'h-4 w-4',
                  action === 'reactivating' && 'animate-spin',
                )}
              />
              {action === 'reactivating' ?
                'Keeping subscription...'
              : 'Keep subscription active'}
            </button>
          : <button
              onClick={() => void handleCancel()}
              disabled={action !== 'idle' || !canScheduleCancel}
              className={cn(
                'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300',
                'border-destructive/40 bg-destructive/8 text-destructive hover:bg-destructive/12',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              <LuCalendarX className="h-4 w-4" />
              {action === 'canceling' ? 'Canceling...' : 'Cancel at period end'}
            </button>
          }
          <button
            onClick={handleBack}
            className="border-foreground/15 bg-background/40 text-foreground hover:border-brand/30 hover:bg-brand/10 flex cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300"
          >
            {isCancelScheduled ? 'Back to billing' : 'Keep my plan'}
          </button>
        </div>

        {billingState.loading && !billing && (
          <div className="mt-4">
            <LoadingInline
              label="Loading subscription details"
              tone="muted"
              size="sm"
            />
          </div>
        )}
        {!billingState.loading && isGrace && (
          <p className="text-foreground-alt/70 mt-4 text-sm leading-relaxed">
            This subscription is already canceled. If you want cloud access
            again, you can start a new checkout from the plan page.
          </p>
        )}
        {checkout.polling && (
          <div className="border-brand/20 bg-brand/5 mt-4 rounded-lg border px-3 py-3 text-sm backdrop-blur-sm">
            <div className="flex items-center gap-2">
              <LuRefreshCw className="text-brand h-4 w-4 animate-spin" />
              <span className="text-foreground">
                Reactivation is in progress. You will return to billing details
                when Stripe confirms the checkout.
              </span>
            </div>
            {checkout.showRetry && (
              <button
                onClick={checkout.continueCheckout}
                className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground mt-3 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
              >
                <LuRefreshCw className="h-3.5 w-3.5" />
                <span>Continue with Stripe</span>
              </button>
            )}
          </div>
        )}
        {(error || checkout.error) && (
          <p className="text-destructive mt-4 text-sm">
            {error || checkout.error}
          </p>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex flex-col items-center gap-1 text-center">
          <h2 className="text-foreground text-lg font-semibold tracking-tight">
            Questions before you cancel?
          </h2>
          <p className="text-foreground-alt text-sm">
            The short version, in plain language.
          </p>
        </div>
        <FaqAccordion items={CANCEL_FAQ} />
      </div>

      <PageFooter />
    </PageWrapper>
  )
}
