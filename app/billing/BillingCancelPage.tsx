/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useState, type ReactNode } from 'react'
import type { IconType } from 'react-icons'
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

type CheckoutState = ReturnType<typeof useBillingAccountCheckout>

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

/** CancelStage is how far the subscription is along the cancellation path. */
type CancelStage = 'active' | 'scheduled' | 'grace'

/** cancelStageFor maps a lifecycle state to its cancellation stage. */
function cancelStageFor(
  lifecycleState: AccountLifecycleState | undefined,
): CancelStage {
  switch (lifecycleState) {
    case AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END:
      return 'scheduled'
    case AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY:
      return 'grace'
    default:
      return 'active'
  }
}

/** formatEndDate formats the plan end timestamp, or returns null without one. */
function formatEndDate(endAt: bigint | number | undefined): string | null {
  if (!endAt) return null
  return new Date(Number(endAt)).toLocaleDateString(undefined, {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

/** cancelBadgeLabel returns the badge text above the page title. */
function cancelBadgeLabel(stage: CancelStage): string {
  switch (stage) {
    case 'scheduled':
      return 'Cancellation scheduled'
    case 'grace':
      return 'Read-only export window'
    case 'active':
      return 'End-of-period cancellation'
  }
}

/** cancelTitle returns the page title for the stage. */
function cancelTitle(stage: CancelStage, endLabel: string | null): string {
  switch (stage) {
    case 'scheduled':
      return endLabel
        ? `Your plan will already cancel on ${endLabel}`
        : 'Your plan is already set to cancel'
    case 'grace':
      return 'Your plan is in the 30-day export window'
    case 'active':
      return 'Cancel your Spacewave Cloud plan?'
  }
}

/** cancelSubtitle returns the page subtitle for the stage. */
function cancelSubtitle(stage: CancelStage): string {
  switch (stage) {
    case 'scheduled':
      return 'Nothing else needs to happen. You still have full access until then. If you changed your mind, you can keep the plan active.'
    case 'grace':
      return 'Your subscription has already ended. Cloud data is read-only for 30 days so you can export what you need or start a new subscription.'
    case 'active':
      return 'This keeps your plan active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export it.'
  }
}

/** undoCopy returns the "Easy to undo" card text for the stage. */
function undoCopy(stage: CancelStage): string {
  switch (stage) {
    case 'grace':
      return 'Start a new subscription whenever you want to restore read and write access.'
    case 'scheduled':
      return 'If you changed your mind, keep the subscription active with one click.'
    case 'active':
      return 'If you change your mind later, you can keep the subscription active before it ends.'
  }
}

/** accessCopy returns the access card text for the stage and end date. */
function accessCopy(stage: CancelStage, endLabel: string | null): string {
  if (stage === 'grace') {
    return 'The subscription has already ended. You can still export existing cloud data during the remaining 30-day window.'
  }
  if (endLabel) return `Everything keeps working normally until ${endLabel}.`
  return 'Everything keeps working normally until the current billing period ends.'
}

/**
 * useCancelActions runs the cancel and keep-active requests, and tracks which
 * one is in flight and its error.
 */
function useCancelActions(checkout: CheckoutState) {
  const navigate = useNavigate()
  const session = SessionContext.useContext().value
  const { billingAccountId } = useBillingStateContext()
  const [action, setAction] = useState<'idle' | 'canceling' | 'reactivating'>(
    'idle',
  )
  const [error, setError] = useState<string | null>(null)

  const cancel = useCallback(async () => {
    if (!session || action !== 'idle') return
    setAction('canceling')
    setError(null)
    try {
      await session.spacewave.cancelSubscription(billingAccountId)
      navigate({ path: '../' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Cancel failed')
      setAction('idle')
    }
  }, [session, action, navigate, billingAccountId])

  const keep = useCallback(async () => {
    if (!session || !billingAccountId || action !== 'idle') return
    setAction('reactivating')
    setError(null)
    try {
      const resp =
        await session.spacewave.reactivateSubscription(billingAccountId)
      if (resp.needsCheckout) {
        await checkout.startCheckout(billingAccountId)
        return
      }
      navigate({ path: '../' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reactivate failed')
      setAction('idle')
    }
  }, [session, action, navigate, billingAccountId, checkout])

  return { action, error, cancel, keep }
}

interface CancelHeaderProps {
  stage: CancelStage
  endLabel: string | null
}

/** CancelHeader renders the logo, stage badge, title, and subtitle. */
function CancelHeader({ stage, endLabel }: CancelHeaderProps) {
  return (
    <div className="flex flex-col items-center gap-2">
      <AnimatedLogo followMouse={false} />
      <div className="border-brand/25 bg-brand/8 text-brand text-metadata mt-2 inline-flex items-center gap-2 rounded-full border px-3 py-1 font-medium tracking-wide uppercase">
        <LuCalendarX className="size-3.5" />
        {cancelBadgeLabel(stage)}
      </div>
      <h1 className="mt-2 text-center text-xl font-semibold tracking-wide">
        {cancelTitle(stage, endLabel)}
      </h1>
      <p className="text-foreground-alt max-w-xl text-center text-sm leading-relaxed">
        {cancelSubtitle(stage)}
      </p>
    </div>
  )
}

/** CancelNextBody explains what happens to access and data after the plan ends. */
function CancelNextBody({ stage, endLabel }: CancelHeaderProps) {
  if (stage === 'grace') {
    return (
      <>
        Your plan has already ended. Cloud data is read-only for 30 days so you
        can export or start a new subscription.
      </>
    )
  }
  if (endLabel) {
    return (
      <>
        You keep full access through{' '}
        <span className="text-foreground font-medium">{endLabel}</span>. After
        that, your cloud data stays read-only for 30 days so you can export what
        you need.
      </>
    )
  }
  return (
    <>
      You keep full access through the end of the current billing period. After
      that, your cloud data stays read-only for 30 days so you can export what
      you need.
    </>
  )
}

/** CancelNextSection renders the "what happens next" heading and body. */
function CancelNextSection({ stage, endLabel }: CancelHeaderProps) {
  return (
    <div className="mb-6 flex items-start gap-3">
      <div className="bg-brand/10 flex size-10 shrink-0 items-center justify-center rounded-lg">
        <LuCalendarX className="text-brand size-5" />
      </div>
      <div className="space-y-2">
        <h2 className="text-foreground text-lg font-semibold">
          {stage === 'active' ? 'If you cancel now' : 'What happens next'}
        </h2>
        <p className="text-foreground-alt text-sm leading-relaxed">
          <CancelNextBody stage={stage} endLabel={endLabel} />
        </p>
      </div>
    </div>
  )
}

interface CancelInfoCardProps {
  icon: IconType
  title: string
  children: ReactNode
}

/** CancelInfoCard renders one outcome card with an icon, title, and text. */
function CancelInfoCard({ icon: Icon, title, children }: CancelInfoCardProps) {
  return (
    <div className="border-foreground/10 bg-background/45 rounded-lg border p-4">
      <div className="mb-2 flex items-center gap-2">
        <div className="bg-brand/10 flex size-8 items-center justify-center rounded-md">
          <Icon className="text-brand size-4" />
        </div>
        <h3 className="text-foreground text-sm font-semibold">{title}</h3>
      </div>
      <p className="text-foreground-alt text-sm leading-relaxed text-balance">
        {children}
      </p>
    </div>
  )
}

/** CancelOutcomeCards renders the access, export window, and undo cards. */
function CancelOutcomeCards({ stage, endLabel }: CancelHeaderProps) {
  return (
    <div className="mt-6 grid gap-3 sm:grid-cols-3">
      <CancelInfoCard
        icon={LuClock3}
        title={stage === 'grace' ? 'Read-only access' : 'Full access'}
      >
        {accessCopy(stage, endLabel)}
      </CancelInfoCard>
      <CancelInfoCard icon={LuDownload} title="30-day export window">
        Cloud data stays read-only for 30 days after the plan ends so you can
        export it safely or re-subscribe.
      </CancelInfoCard>
      <CancelInfoCard icon={LuShield} title="Easy to undo">
        {undoCopy(stage)}
      </CancelInfoCard>
    </div>
  )
}

/** CancelDeletionNote points to account deletion for people who want that. */
function CancelDeletionNote() {
  return (
    <div className="border-foreground/10 bg-background/45 mt-6 rounded-lg border p-4">
      <div className="flex items-start gap-3">
        <div className="bg-destructive/10 flex size-8 shrink-0 items-center justify-center rounded-md">
          <LuTriangleAlert className="text-destructive size-4" />
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
            <LuTrash2 className="size-3.5" />
            Go to account settings and choose{' '}
            <span className="text-foreground font-medium">
              Delete account
            </span>{' '}
            if that is the path you want.
          </div>
        </div>
      </div>
    </div>
  )
}

interface CancelActionsProps {
  stage: CancelStage
  canScheduleCancel: boolean
  action: 'idle' | 'canceling' | 'reactivating'
  polling: boolean
  onKeep: () => void
  onCancel: () => void
  onBack: () => void
}

/** CancelActions renders the primary keep or cancel button and the back button. */
function CancelActions({
  stage,
  canScheduleCancel,
  action,
  polling,
  onKeep,
  onCancel,
  onBack,
}: CancelActionsProps) {
  return (
    <div className="mt-8 flex flex-col gap-3 sm:flex-row">
      {stage === 'scheduled' ? (
        <button
          type="button"
          onClick={onKeep}
          disabled={action !== 'idle' || polling}
          className={cn(
            'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300',
            'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <LuRefreshCw
            className={cn(
              'size-4',
              action === 'reactivating' && 'animate-spin',
            )}
          />
          {action === 'reactivating'
            ? 'Keeping subscription…'
            : 'Keep subscription active'}
        </button>
      ) : (
        <button
          type="button"
          onClick={onCancel}
          disabled={action !== 'idle' || !canScheduleCancel}
          className={cn(
            'flex cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300',
            'border-destructive/40 bg-destructive/8 text-destructive hover:bg-destructive/12',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <LuCalendarX className="size-4" />
          {action === 'canceling' ? 'Canceling…' : 'Cancel at period end'}
        </button>
      )}
      <button
        type="button"
        onClick={onBack}
        className="border-foreground/15 bg-background/40 text-foreground hover:border-brand/30 hover:bg-brand/10 flex cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition duration-300"
      >
        {stage === 'scheduled' ? 'Back to billing' : 'Keep my plan'}
      </button>
    </div>
  )
}

/** CancelCheckoutNotice shows the reactivation checkout progress and retry. */
function CancelCheckoutNotice({ checkout }: { checkout: CheckoutState }) {
  return (
    <div className="border-brand/20 bg-brand/5 mt-4 rounded-lg border p-3 text-sm backdrop-blur-sm">
      <div className="flex items-center gap-2">
        <LuRefreshCw className="text-brand size-4 animate-spin" />
        <span className="text-foreground">
          Reactivation is in progress. You will return to billing details when
          Stripe confirms the checkout.
        </span>
      </div>
      {checkout.showRetry && (
        <button
          type="button"
          onClick={checkout.continueCheckout}
          className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground mt-3 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
        >
          <LuRefreshCw className="size-3.5" />
          <span>Continue with Stripe</span>
        </button>
      )}
    </div>
  )
}

interface CancelNoticesProps {
  stage: CancelStage
  loading: boolean
  loaded: boolean
  error: string | null
  checkout: CheckoutState
}

/** CancelNotices renders the loading, grace, checkout, and error notices. */
function CancelNotices({
  stage,
  loading,
  loaded,
  error,
  checkout,
}: CancelNoticesProps) {
  const displayError = error || checkout.error

  return (
    <>
      {loading && !loaded && (
        <div className="mt-4">
          <LoadingInline
            label="Loading subscription details"
            tone="muted"
            size="sm"
          />
        </div>
      )}
      {!loading && stage === 'grace' && (
        <p className="text-foreground-alt/70 mt-4 text-sm leading-relaxed">
          This subscription is already canceled. If you want cloud access again,
          you can start a new checkout from the plan page.
        </p>
      )}
      {checkout.polling && <CancelCheckoutNotice checkout={checkout} />}
      {displayError && (
        <p className="text-destructive mt-4 text-sm">{displayError}</p>
      )}
    </>
  )
}

/** CancelFaq renders the cancellation questions. */
function CancelFaq() {
  return (
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
  )
}

// BillingCancelPage explains cancellation outcomes before confirming.
export function BillingCancelPage() {
  const navigate = useNavigate()
  const billingState = useBillingStateContext()
  const checkout = useBillingAccountCheckout({
    onCompleted: () => navigate({ path: '../' }),
  })
  const { action, error, cancel, keep } = useCancelActions(checkout)

  const billing = billingState.response?.billingAccount
  const lifecycleState = billing?.lifecycleState
  const stage = cancelStageFor(lifecycleState)
  const canScheduleCancel =
    lifecycleState === AccountLifecycleState.AccountLifecycleState_ACTIVE ||
    stage === 'scheduled'
  const endLabel = formatEndDate(billing?.cancelAt || billing?.currentPeriodEnd)

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  return (
    <PageWrapper
      backButton={
        <button
          type="button"
          onClick={handleBack}
          className="text-foreground-alt hover:text-brand flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="size-4" />
          Back to billing
        </button>
      }
    >
      {checkout.consentDialog}
      <CancelHeader stage={stage} endLabel={endLabel} />

      <div className="border-brand/20 bg-background-card/55 overflow-hidden rounded-xl border p-8 backdrop-blur-sm">
        <CancelNextSection stage={stage} endLabel={endLabel} />
        <CancelOutcomeCards stage={stage} endLabel={endLabel} />
        <CancelDeletionNote />
        <CancelActions
          stage={stage}
          canScheduleCancel={canScheduleCancel}
          action={action}
          polling={checkout.polling}
          onKeep={() => void keep()}
          onCancel={() => void cancel()}
          onBack={handleBack}
        />
        <CancelNotices
          stage={stage}
          loading={billingState.loading}
          loaded={!!billing}
          error={error}
          checkout={checkout}
        />
      </div>

      <CancelFaq />
      <PageFooter />
    </PageWrapper>
  )
}
