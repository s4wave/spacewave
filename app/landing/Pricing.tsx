import { useCallback, useState } from 'react'
import { LuCheck, LuCloud, LuCloudOff, LuPlus } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LegalPageLayout } from './LegalPageLayout.js'
import {
  FREE_FEATURES,
  CLOUD_FEATURES,
  OVERAGE_ITEMS,
  PLAN_PRICE_MONTHLY,
} from '../provider/spacewave/pricing.js'

export const metadata = {
  title: 'Spacewave Pricing',
  description:
    'Spacewave is free forever. Add cloud sync, backup, and collaboration for $8/month. 100 GB encrypted storage. Cancel anytime.',
  canonicalPath: '/pricing',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
  jsonLd: {
    '@context': 'https://schema.org',
    '@type': 'Product',
    name: 'Spacewave Cloud',
    description: 'Cloud sync and backup for Spacewave',
    offers: [
      { '@type': 'Offer', price: '0', priceCurrency: 'USD', name: 'Free' },
      {
        '@type': 'Offer',
        price: '8',
        priceCurrency: 'USD',
        name: 'Cloud',
        billingPeriod: 'P1M',
      },
    ],
  },
}

const FAQ_ITEMS = [
  {
    question: 'Is the free tier really unlimited?',
    answer:
      'Yes. Spacewave runs entirely on your devices with no server dependency. All features work locally, including plugins, encryption, and peer-to-peer sync. There is no credit card required and no time limit.',
  },
  {
    question: 'What does Cloud add?',
    answer:
      'Cloud provides always-on sync and backup through our infrastructure. Your devices sync through the cloud when direct peer-to-peer is unavailable, and your data is backed up so you never lose it.',
  },
  {
    question: 'What happens to my data if I cancel Cloud?',
    answer:
      'Your local data stays on your devices and is always yours. Cloud-synced data has a 30-day read-only grace period after cancellation. You can export everything at any time.',
  },
  {
    question: 'Can I self-host the cloud parts?',
    answer:
      'Yes. Spacewave is open-source and you can run your own relay and storage infrastructure. The Cloud plan is for those who want managed infrastructure without running servers.',
  },
  {
    question: 'How does usage-based pricing work?',
    answer: (
      <>
        Your Cloud plan includes a generous baseline:{' '}
        <strong className="text-foreground">100 GB</strong> cloud storage, 1
        million writes, and 10 million cloud reads per month. Most users never
        exceed this. If you do, overages are billed at cost-plus rates with no
        surprises.
      </>
    ),
  },
] as const

// FeatureList renders a list of features with check marks.
function FeatureList({ features }: { features: string[] }) {
  return (
    <ul className="flex flex-col gap-3">
      {features.map((feature) => (
        <li key={feature} className="flex items-start gap-2">
          <LuCheck className="text-brand mt-0.5 h-4 w-4 shrink-0" />
          <span className="text-foreground-alt text-sm">{feature}</span>
        </li>
      ))}
    </ul>
  )
}

// FaqItem renders an expandable FAQ question and answer as a standalone card.
function FaqItem({
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
        'group cursor-pointer rounded-lg border p-5 backdrop-blur-sm transition-all',
        isOpen ?
          'border-foreground/12 bg-background-card/60'
        : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:-translate-y-0.5',
      )}
      onClick={onToggle}
    >
      <div className="flex items-start justify-between gap-4">
        <h3
          className={cn(
            'text-sm leading-relaxed font-semibold transition-colors @lg:text-base',
            isOpen ? 'text-foreground' : (
              'text-foreground group-hover:text-brand'
            ),
          )}
        >
          {question}
        </h3>
        <div
          className={cn(
            'mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md transition-all',
            isOpen ?
              'bg-brand/12 text-brand rotate-45'
            : 'bg-foreground/6 text-foreground-alt group-hover:bg-brand/8 group-hover:text-brand',
          )}
        >
          <LuPlus className="h-3 w-3" />
        </div>
      </div>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <p className="text-foreground-alt pt-3 text-sm leading-relaxed">
            {answer}
          </p>
        </div>
      </div>
    </div>
  )
}

// FaqSection renders the pricing FAQ as an accordion matching homepage style.
function FaqSection() {
  const [openIndex, setOpenIndex] = useState<number>(0)
  const handleToggle = useCallback((index: number) => {
    setOpenIndex((prev) => (prev === index ? -1 : index))
  }, [])

  return (
    <section className="relative z-10 mx-auto w-full max-w-4xl px-4 py-14 @lg:px-8 @lg:py-16">
      <span className="text-foreground-alt mb-8 block text-center text-xs font-semibold tracking-[0.2em] uppercase">
        Frequently asked questions
      </span>
      <div className="flex flex-col gap-3">
        {FAQ_ITEMS.map((item, index) => (
          <FaqItem
            key={item.question}
            question={item.question}
            answer={item.answer}
            isOpen={index === openIndex}
            onToggle={() => handleToggle(index)}
          />
        ))}
      </div>
    </section>
  )
}

// Pricing renders the pricing page.
export function Pricing() {
  const navigate = useNavigate()
  const goToQuickstart = useCallback(() => {
    navigate({ path: '/quickstart/local' })
  }, [navigate])
  const goToLogin = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  return (
    <LegalPageLayout
      title="Spacewave Pricing"
      subtitle="Spacewave is free forever. Cloud sync and backup when you need it."
    >
      {/* Pricing cards */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <div className="grid gap-6 @lg:grid-cols-2">
          {/* Cloud tier */}
          <div className="border-brand/30 bg-background-card/50 relative flex flex-col rounded-lg border-2 p-6 backdrop-blur-sm @lg:p-8">
            <div className="bg-brand absolute -top-3 right-4 rounded-full px-3 py-0.5 text-xs font-semibold text-black">
              Popular
            </div>
            <div className="mb-4 flex items-center gap-2">
              <LuCloud className="text-brand h-5 w-5" />
              <h2 className="text-foreground text-xl font-bold">Cloud</h2>
            </div>
            <div className="mt-2 mb-2 flex items-baseline gap-1">
              <span className="text-foreground text-4xl font-bold">
                ${PLAN_PRICE_MONTHLY}
              </span>
              <span className="text-foreground-alt text-sm">/ month</span>
            </div>
            <p className="text-foreground-alt mb-6 text-sm leading-relaxed">
              Always-on sync, backup, and collaboration.
            </p>
            <FeatureList features={CLOUD_FEATURES} />
            <div className="flex-1" />
            <button
              onClick={goToLogin}
              className="border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15 mt-8 flex cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5"
            >
              Get Started
            </button>
          </div>

          {/* Free tier */}
          <div className="border-foreground/8 bg-background-card/50 flex flex-col rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <div className="mb-4 flex items-center gap-2">
              <LuCloudOff className="text-foreground-alt h-5 w-5" />
              <h2 className="text-foreground text-xl font-bold">Free</h2>
            </div>
            <div className="mt-2 mb-2 flex items-baseline gap-1">
              <span className="text-foreground text-4xl font-bold">$0</span>
              <span className="text-foreground-alt text-sm">/ forever</span>
            </div>
            <p className="text-foreground-alt mb-6 text-sm leading-relaxed">
              The full app, running on your devices.
            </p>
            <FeatureList features={FREE_FEATURES} />
            <div className="flex-1" />
            <button
              onClick={goToQuickstart}
              className="border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8 mt-8 flex cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5"
            >
              Get Started
            </button>
          </div>
        </div>
      </section>

      {/* Overage table */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <span className="text-foreground-alt mb-6 block text-center text-xs font-semibold tracking-[0.2em] uppercase">
          Usage above baseline
        </span>
        <div className="border-foreground/8 bg-background-card/50 overflow-hidden rounded-lg border backdrop-blur-sm">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-foreground/8 border-b">
                <th className="text-foreground-alt px-6 py-3 text-left font-medium">
                  Resource
                </th>
                <th className="text-foreground-alt px-6 py-3 text-right font-medium">
                  Baseline
                </th>
                <th className="text-foreground-alt px-6 py-3 text-right font-medium">
                  Overage rate
                </th>
              </tr>
            </thead>
            <tbody>
              {OVERAGE_ITEMS.map((item) => (
                <tr
                  key={item.resource}
                  className="border-foreground/5 border-b last:border-b-0"
                >
                  <td className="text-foreground px-6 py-3">{item.resource}</td>
                  <td className="text-foreground-alt px-6 py-3 text-right font-mono text-xs">
                    {item.baseline}
                  </td>
                  <td className="text-foreground-alt px-6 py-3 text-right font-mono text-xs">
                    {item.rate}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="text-foreground-alt/60 mt-3 text-center text-xs">
          Most users stay well within the included baseline. No hidden fees.
        </p>
      </section>

      {/* Trust signals */}
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-8 @lg:px-8">
        <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-6 text-xs">
          <span className="flex items-center gap-1.5">
            <LuCheck className="text-brand h-3.5 w-3.5" />
            Cancel anytime
          </span>
          <span className="flex items-center gap-1.5">
            <LuCheck className="text-brand h-3.5 w-3.5" />
            No hidden fees
          </span>
          <span className="flex items-center gap-1.5">
            <LuCheck className="text-brand h-3.5 w-3.5" />
            Open-source
          </span>
        </div>
      </section>

      {/* Divider */}
      <div className="via-foreground/8 mx-auto h-px w-full max-w-4xl bg-gradient-to-r from-transparent to-transparent" />

      {/* FAQ */}
      <FaqSection />
    </LegalPageLayout>
  )
}
