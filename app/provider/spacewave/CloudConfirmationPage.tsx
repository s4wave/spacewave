import { useState, useCallback } from 'react'
import {
  LuArrowLeft,
  LuCheck,
  LuCloud,
  LuCode,
  LuDatabase,
  LuGithub,
  LuGlobe,
  LuHardDrive,
  LuPlus,
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
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

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

const FOOTER_LINKS = [
  { href: '#/tos', label: 'Terms' },
  { href: '#/dmca', label: 'DMCA' },
  { href: '#/privacy', label: 'Privacy' },
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

export const CLOUD_FAQ: { question: string; answer: React.ReactNode }[] = [
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
          <LuArrowLeft className="h-4 w-4" />
          Back to plan selection
        </button>
      }
    >
      {/* Header */}
      <div className="flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <h1 className="mt-2 text-xl font-bold tracking-wide">
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
            <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-lg">
              <LuCloud className="text-brand h-5 w-5" />
            </div>
            <div>
              <h2 className="text-foreground text-lg font-bold">Cloud</h2>
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
            {loading ?
              <>
                <Spinner />
                {polling ?
                  'Activating subscription...'
                : 'Continuing with Stripe...'}
              </>
            : 'Continue with Stripe...'}
          </button>
          {showRetry && (
            <button
              onClick={onRetry}
              className="border-brand bg-brand/10 text-foreground hover:bg-brand/20 flex cursor-pointer items-center justify-center rounded-r-md border border-l-0 px-3 transition-all duration-300"
              title="Retry"
            >
              <LuRefreshCw className="h-4 w-4" />
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
            <LuCheck className="text-brand h-3.5 w-3.5" />
            {text}
          </span>
        ))}
      </div>

      {/* Cloud FAQ */}
      <FaqAccordion items={CLOUD_FAQ} />

      {/* Open source footer */}
      <div className="border-foreground/6 flex flex-col items-center justify-between gap-4 rounded-lg border bg-gradient-to-r from-blue-500/5 via-indigo-500/5 to-cyan-500/5 px-6 py-5 backdrop-blur-sm sm:flex-row">
        <div className="flex items-center gap-3">
          <div className="rounded-lg bg-blue-500/10 p-2.5">
            <LuCode className="text-brand h-5 w-5" />
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
          className="group border-foreground/15 bg-background/50 text-foreground hover:border-brand/30 hover:bg-brand/10 flex items-center rounded-md border px-4 py-1.5 text-xs font-medium transition-all duration-300"
        >
          <LuGithub className="mr-1.5 h-3.5 w-3.5 transition-transform duration-300 group-hover:scale-110" />
          <span className="select-none">View on GitHub</span>
        </a>
      </div>

      <PageFooter />
    </PageWrapper>
  )
}

// PageWrapper renders the shared outer layout for plan pages.
export function PageWrapper({
  backButton,
  children,
}: {
  backButton?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
      {backButton && (
        <div className="relative z-10 w-full max-w-2xl">{backButton}</div>
      )}
      <div
        className={cn(
          'relative z-10 my-auto flex w-full max-w-2xl flex-col gap-6',
          backButton && 'pt-6',
        )}
      >
        {children}
      </div>
    </div>
  )
}

// FeatureGrid renders icon+text feature items in a two-column grid.
export function FeatureGrid({
  features,
}: {
  features: {
    icon: React.ComponentType<{ className?: string }>
    text: string
  }[]
}) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {features.map((feature) => (
        <div key={feature.text} className="flex items-start gap-2">
          <feature.icon className="text-brand mt-0.5 h-4 w-4 shrink-0" />
          <span className="text-foreground-alt text-sm">{feature.text}</span>
        </div>
      ))}
    </div>
  )
}

// PageFooter renders the bottom attribution and legal links.
export function PageFooter() {
  return (
    <p className="text-foreground-alt/40 pt-4 pb-2 text-center text-xs">
      <span className="block">
        Spacewave Cloud by{' '}
        <a
          href="https://aperture.us"
          target="_blank"
          rel="noopener noreferrer"
          className="text-foreground/50 hover:text-foreground/70 transition-colors"
        >
          Aperture Robotics
        </a>
        , LLC. powered by Cloudflare
      </span>
      <span className="mt-1 block">
        {FOOTER_LINKS.map((link, index) => (
          <span key={link.href}>
            {index > 0 && <span className="mx-1.5">|</span>}
            <a
              href={link.href}
              className="text-foreground/50 hover:text-foreground/70 transition-colors"
            >
              {link.label}
            </a>
          </span>
        ))}
      </span>
    </p>
  )
}

// PlanFaqItem renders a single expandable FAQ item.
export function PlanFaqItem({
  question,
  answer,
  isOpen,
  onToggle,
}: {
  question: string
  answer: React.ReactNode
  isOpen: boolean
  onToggle: () => void
}) {
  return (
    <div
      className={cn(
        'cursor-pointer rounded-lg border p-4 backdrop-blur-sm transition-all',
        isOpen ?
          'border-foreground/12 bg-background-card/60'
        : 'border-foreground/6 bg-background-card/20 hover:border-foreground/12',
      )}
      onClick={onToggle}
    >
      <div className="flex items-start justify-between gap-3">
        <h3
          className={cn(
            'text-xs leading-relaxed font-medium transition-colors',
            isOpen ? 'text-foreground' : (
              'text-foreground-alt group-hover:text-foreground'
            ),
          )}
        >
          {question}
        </h3>
        <div
          className={cn(
            'mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded transition-all',
            isOpen ?
              'bg-brand/12 text-brand rotate-45'
            : 'bg-foreground/6 text-foreground-alt',
          )}
        >
          <LuPlus className="h-2.5 w-2.5" />
        </div>
      </div>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <p className="text-foreground-alt pt-2 text-xs leading-relaxed">
            {answer}
          </p>
        </div>
      </div>
    </div>
  )
}

// FaqAccordion renders a list of FAQ items as an accordion.
export function FaqAccordion({
  items,
}: {
  items: { question: string; answer: React.ReactNode }[]
}) {
  const [openIndex, setOpenIndex] = useState<number>(-1)
  const handleToggle = useCallback((index: number) => {
    setOpenIndex((prev) => (prev === index ? -1 : index))
  }, [])

  return (
    <div className="flex flex-col gap-2">
      {items.map((item, index) => (
        <PlanFaqItem
          key={item.question}
          question={item.question}
          answer={item.answer}
          isOpen={index === openIndex}
          onToggle={() => handleToggle(index)}
        />
      ))}
    </div>
  )
}
