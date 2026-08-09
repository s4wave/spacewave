import type { ReactNode } from 'react'
import {
  LuArrowLeft,
  LuCheck,
  LuCloud,
  LuCode,
  LuDatabase,
  LuGithub,
  LuGlobe,
  LuHardDrive,
  LuRefreshCw,
  LuServer,
  LuShield,
  LuSmartphone,
  LuUsers,
  LuZap,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { cn } from '@s4wave/web/style/utils.js'
import {
  PLAN_PRICE_MONTHLY,
  OVERAGE_STORAGE_PER_GB,
  OVERAGE_WRITE_PER_MILLION,
  OVERAGE_READ_PER_MILLION,
} from '@s4wave/app/provider/spacewave/pricing.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import { FaqAccordion } from './FaqAccordion.js'
import { FeatureGrid } from './FeatureGrid.js'
import { PageFooter } from './PageFooter.js'
import { PageWrapper } from './PageWrapper.js'

const CLOUD_EXPANDED_FEATURES = [
  { icon: LuGlobe, text: 'Cloud sync and backup across all devices' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuServer, text: '100 GB cloud storage included' },
  { icon: LuZap, text: '1M writes / 10M cloud reads per month' },
  {
    icon: LuShield,
    text: 'End-to-end encrypted privacy',
  },
  { icon: LuSmartphone, text: 'Access from any device, anywhere' },
  { icon: LuDatabase, text: 'Automatic backups, high speed' },
  { icon: LuHardDrive, text: 'Works offline, syncs when reconnected' },
]

const TRUST_SIGNALS = [
  'Cancel anytime',
  'No hidden fees',
  '30-day export window',
  'Open-source',
]

const E2E_ENCRYPTION_LINK = (
  <a
    href="https://www.cloudflare.com/learning/privacy/what-is-end-to-end-encryption/"
    target="_blank"
    rel="noopener noreferrer"
    className="text-brand hover:underline"
    onClick={(e) => e.stopPropagation()}
  >
    a standard approach to data protection
  </a>
)

const CANCEL_FAQ_ANSWER =
  'Yes. Standard cancellation keeps your subscription active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export what you need or re-subscribe. If you want to fully delete your account, that is handled separately and requires email verification.'

const OVERAGE_FAQ_ANSWER = `Overages at very low prices: $${OVERAGE_STORAGE_PER_GB.toFixed(2)}/GB-month storage, $${OVERAGE_WRITE_PER_MILLION.toFixed(2)}/million writes, $${OVERAGE_READ_PER_MILLION.toFixed(2)}/million cloud reads. You can monitor your usage anytime. Limit resets every month.`

export const CLOUD_FAQ: { question: string; answer: ReactNode }[] = [
  {
    question: 'Can I cancel my subscription?',
    answer: CANCEL_FAQ_ANSWER,
  },
  {
    question: 'What if I go over the baseline?',
    answer: OVERAGE_FAQ_ANSWER,
  },
  {
    question: 'Is my payment secure?',
    answer:
      'Payments are processed by Stripe, the industry standard for secure payment processing. We never see or store your card details.',
  },
  {
    question: 'Can I migrate my data later?',
    answer:
      'Yes. You can export your data or migrate between Cloud and Local at any time. Your data is always yours.',
  },
  {
    question: 'Is my data encrypted on Cloud?',
    answer: (
      <>
        Yes. Your data is end-to-end encrypted before leaving your device. Even
        on our servers, we cannot read your content. This is{' '}
        {E2E_ENCRYPTION_LINK} on the web.
      </>
    ),
  },
]

// CloudConfirmationPageProps are the props for CloudConfirmationPage.
export interface CloudConfirmationPageProps {
  loading: boolean
  polling: boolean
  showRetry: boolean
  error: string | null
  root: boolean
  checkoutUrl?: string
  onBack: () => void
  onRetry: () => void
  onLoading?: () => void
}

// CloudConfirmationPage renders the expanded cloud confirmation view.
export function CloudConfirmationPage({
  loading,
  polling,
  showRetry,
  error,
  root,
  checkoutUrl,
  onBack,
  onRetry,
  onLoading,
}: CloudConfirmationPageProps) {
  return (
    <PageWrapper
      backButton={
        <button
          onClick={onBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="size-4" />
          Back to plan selection
        </button>
      }
    >
      {/* Header */}
      <div className="flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <h1 className="mt-2 text-xl font-semibold tracking-wide">
          Spacewave Cloud
        </h1>
        <p className="text-foreground-alt text-center text-sm">
          Always-on sync, backup, and collaboration
        </p>
      </div>

      {/* Expanded cloud card */}
      <div className="border-brand/30 bg-background-card/50 overflow-hidden rounded-lg border p-8 backdrop-blur-sm">
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="bg-brand/10 flex size-10 items-center justify-center rounded-lg">
              <LuCloud className="text-brand size-5" />
            </div>
            <div>
              <h2 className="text-foreground text-lg font-semibold">Cloud</h2>
              <p className="text-foreground-alt text-xs">Everything you need</p>
            </div>
          </div>
          <div className="flex items-baseline gap-1">
            <span className="text-foreground text-3xl font-bold">
              ${PLAN_PRICE_MONTHLY}
            </span>
            <span className="text-foreground-alt text-sm">/ month</span>
          </div>
        </div>

        <FeatureGrid features={CLOUD_EXPANDED_FEATURES} />

        {/* Checkout button */}
        <div className="mt-8 flex gap-1">
          <button
            onClick={() => {
              if (showRetry && checkoutUrl) {
                window.open(checkoutUrl, '_blank')
                onLoading?.()
              } else {
                onRetry()
              }
            }}
            disabled={loading || !root}
            className={cn(
              'flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
              'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              showRetry ? 'rounded-r-none' : '',
            )}
          >
            {loading ? (
              <>
                <Spinner />
                {polling
                  ? 'Activating subscription…'
                  : 'Continuing with Stripe…'}
              </>
            ) : (
              'Continue with Stripe…'
            )}
          </button>
          {showRetry && (
            <button
              onClick={onRetry}
              className="border-brand bg-brand/10 text-foreground hover:bg-brand/20 flex cursor-pointer items-center justify-center rounded-r-md border border-l-0 px-3 transition duration-300"
              title="Retry"
            >
              <LuRefreshCw className="size-4" />
            </button>
          )}
        </div>

        {error && (
          <p className="text-destructive mt-3 text-center text-xs">{error}</p>
        )}
      </div>

      {/* Trust signals */}
      <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-6 text-xs">
        {TRUST_SIGNALS.map((text) => (
          <span key={text} className="flex items-center gap-1.5">
            <LuCheck className="text-brand size-3.5" />
            {text}
          </span>
        ))}
      </div>

      {/* Cloud FAQ */}
      <FaqAccordion items={CLOUD_FAQ} />

      {/* Open source footer */}
      <div className="border-foreground/6 via-brand/5 flex flex-col items-center justify-between gap-4 rounded-lg border px-6 py-5 backdrop-blur-sm sm:flex-row">
        <div className="flex items-center gap-3">
          <div className="rounded-lg bg-blue-500/10 p-2.5">
            <LuCode className="text-brand size-5" />
          </div>
          <div>
            <h3 className="text-foreground text-sm font-semibold">
              Open Source Software
            </h3>
            <p className="text-foreground-alt text-xs">
              Built in the open, for everyone
            </p>
          </div>
        </div>
        <a
          href="https://github.com/aperturerobotics"
          target="_blank"
          rel="noopener noreferrer"
          className="group border-foreground/15 bg-background/50 text-foreground hover:border-brand/30 hover:bg-brand/10 flex items-center rounded-md border px-4 py-1.5 text-xs font-medium transition duration-300"
        >
          <LuGithub className="mr-1.5 size-3.5 transition-transform duration-300 group-hover:scale-110" />
          <span className="select-none">View on GitHub</span>
        </a>
      </div>

      <PageFooter />
    </PageWrapper>
  )
}
