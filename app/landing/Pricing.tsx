import { useCallback, useState } from 'react'
import {
  LuCheck,
  LuCloud,
  LuCloudOff,
  LuPlus,
} from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { LegalPageLayout } from './LegalPageLayout.js'
import {
  FREE_FEATURES,
  CLOUD_FEATURES,
  OVERAGE_ITEMS,
  PLAN_PRICE_MONTHLY,
  STORAGE_BASELINE_GB,
  STORAGE_RATE_DISPLAY,
} from '../provider/spacewave/pricing.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { color: themeColors['--color-foreground'] },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': 'var(--leading-relaxed)',
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule7: {
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule9: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
    rotate: '45deg',
  },
  rule10: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
  },
  rule11: {
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    display: 'grid',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
  },
  rule12: { gridTemplateRows: '1fr', opacity: 1 },
  rule13: { gridTemplateRows: '0fr', opacity: 0 },
  rule14: { overflow: 'hidden' },
  rule15: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    marginInline: 'auto',
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    width: '100%',
  },
  rule17: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule18: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: '100%',
  },
  rule19: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule20: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '2px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
  },
  rule21: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: themeColors['--color-brand'],
    color: 'var(--color-black)',
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * -3)`,
  },
  rule22: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule23: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule24: {
    gap: themeLayout['--spacing'],
    alignItems: 'baseline',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-4xl)',
    fontWeight: 'var(--font-weight-bold)',
    lineHeight: 'var(--tw-leading, var(--text-4xl--line-height))',
  },
  rule26: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule27: { flex: 1 },
  rule28: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule29: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule30: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule31: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule32: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule33: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    width: '100%',
  },
  rule34: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule35: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground-alt'],
    fontWeight: themeFonts['--font-weight-medium'],
    textAlign: 'left',
  },
  rule36: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground-alt'],
    fontWeight: themeFonts['--font-weight-medium'],
    textAlign: 'right',
  },
  rule37: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
  },
  rule38: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground'],
  },
  rule39: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule40: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule41: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
    width: '100%',
  },
  rule42: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule43: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule44: {
    marginInline: 'auto',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
    height: '1px',
    maxWidth: 'var(--container-4xl)',
    width: '100%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    cursor: 'pointer',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule3: {
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule5: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export const metadata = {
  title: 'Spacewave Pricing',
  description:
    'Spacewave is free forever. Add cloud sync, backup, and collaboration for $8/month. 100 GiB encrypted storage. Cancel anytime.',
  canonicalPath: '/pricing',
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
        priceSpecification: {
          '@type': 'UnitPriceSpecification',
          price: '8',
          priceCurrency: 'USD',
          billingDuration: 'P1M',
        },
      },
    ],
  },
}

const FAQ_ITEMS = [
  {
    question: 'Is the free tier really unlimited?',
    answer:
      'Yes. Spacewave runs on your devices, and every local feature works without an account, including plugins, encryption, and peer-to-peer sync. There is no credit card required and no time limit. Cloud sync, backup, and sign-in use our servers.',
  },
  {
    question: 'What does Cloud add?',
    answer:
      'Cloud provides always-on sync and backup through our infrastructure. Your devices sync through the cloud when direct peer-to-peer is unavailable, and your cloud data is there on any device you sign in to. Keep your own copy of anything you cannot afford to lose.',
  },
  {
    question: 'What happens to my data if I cancel Cloud?',
    answer:
      'Your local data stays on your devices and is always yours. Cloud-synced data has a 30-day read-only grace period after cancellation. You can export everything at any time.',
  },
  {
    question: 'Can I run without the cloud?',
    answer:
      'Yes. The Spacewave app is open source, and your devices can store and sync your data peer to peer without any server of ours. The Cloud plan adds our managed sync, backup, and relay service, which is not open source.',
  },
  {
    question: 'How does usage-based pricing work?',
    answer: (
      <>
        Your Cloud plan includes{' '}
        <strong sx={stylexStyles3.rule1}>{STORAGE_BASELINE_GB} GiB</strong> of
        cloud storage. Storage above it costs {STORAGE_RATE_DISPLAY}, measured
        hourly, up to the monthly spending limit you choose. Reads and writes
        are not billed. When extra storage is off or at its limit, uploads pause
        until the period renews or you raise the limit.
      </>
    ),
  },
] as const

// FeatureList renders a list of features with check marks.
function FeatureList({ features }: { features: string[] }) {
  return (
    <ul sx={stylexStyles3.rule2}>
      {features.map((feature) => (
        <li key={feature} sx={stylexStyles3.rule3}>
          <LuCheck sx={stylexStyles.rule1} />
          <span sx={stylexStyles3.rule4}>{feature}</span>
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
  const handleFaqKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onToggle()
  }

  return (
    <div
      role="button"
      tabIndex={0}
      sx={[
        [stylexStyles2.rule1, groupMarker],
        isOpen ? stylexStyles2.rule2 : stylexStyles2.rule3,
      ]}
      onClick={onToggle}
      onKeyDown={handleFaqKeyDown}
    >
      <div sx={stylexStyles3.rule5}>
        <h3
          sx={[
            stylexStyles3.rule6,
            isOpen ? stylexStyles3.rule1 : stylexStyles3.rule7,
          ]}
        >
          {question}
        </h3>
        <div
          sx={[
            stylexStyles3.rule8,
            isOpen ? stylexStyles3.rule9 : stylexStyles3.rule10,
          ]}
        >
          <LuPlus sx={stylexStyles.rule2} />
        </div>
      </div>
      <div
        sx={[
          stylexStyles3.rule11,
          isOpen ? stylexStyles3.rule12 : stylexStyles3.rule13,
        ]}
      >
        <div sx={stylexStyles3.rule14}>
          <p sx={stylexStyles3.rule15}>{answer}</p>
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
    <section sx={stylexStyles3.rule16}>
      <span sx={stylexStyles3.rule17}>Frequently asked questions</span>
      <div sx={stylexStyles3.rule2}>
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
      <section sx={stylexStyles3.rule18}>
        <div sx={stylexStyles3.rule19}>
          {/* Cloud tier */}
          <div sx={stylexStyles3.rule20}>
            <div sx={stylexStyles3.rule21}>Popular</div>
            <div sx={stylexStyles3.rule22}>
              <LuCloud sx={stylexStyles.rule3} />
              <h2 sx={stylexStyles3.rule23}>Cloud</h2>
            </div>
            <div sx={stylexStyles3.rule24}>
              <span sx={stylexStyles3.rule25}>${PLAN_PRICE_MONTHLY}</span>
              <span sx={stylexStyles3.rule4}>/ month</span>
            </div>
            <p sx={stylexStyles3.rule26}>
              Always-on sync, backup, and collaboration.
            </p>
            <FeatureList features={CLOUD_FEATURES} />
            <div sx={stylexStyles3.rule27} />
            <button type="button" onClick={goToLogin} sx={stylexStyles3.rule28}>
              Get Started
            </button>
          </div>

          {/* Free tier */}
          <div sx={stylexStyles3.rule29}>
            <div sx={stylexStyles3.rule22}>
              <LuCloudOff sx={stylexStyles.rule4} />
              <h2 sx={stylexStyles3.rule23}>Free</h2>
            </div>
            <div sx={stylexStyles3.rule24}>
              <span sx={stylexStyles3.rule25}>$0</span>
              <span sx={stylexStyles3.rule4}>/ forever</span>
            </div>
            <p sx={stylexStyles3.rule26}>
              The full app, running on your devices.
            </p>
            <FeatureList features={FREE_FEATURES} />
            <div sx={stylexStyles3.rule27} />
            <button
              type="button"
              onClick={goToQuickstart}
              sx={stylexStyles3.rule30}
            >
              Get Started
            </button>
          </div>
        </div>
      </section>

      {/* Overage table */}
      <section sx={stylexStyles3.rule18}>
        <span sx={stylexStyles3.rule31}>Usage above baseline</span>
        <div sx={stylexStyles3.rule32}>
          <table sx={stylexStyles3.rule33}>
            <thead>
              <tr sx={stylexStyles3.rule34}>
                <th sx={stylexStyles3.rule35}>Resource</th>
                <th sx={stylexStyles3.rule36}>Baseline</th>
                <th sx={stylexStyles3.rule36}>Overage rate</th>
              </tr>
            </thead>
            <tbody>
              {OVERAGE_ITEMS.map((item) => (
                <tr key={item.resource} sx={stylexStyles3.rule37}>
                  <td sx={stylexStyles3.rule38}>{item.resource}</td>
                  <td sx={stylexStyles3.rule39}>{item.baseline}</td>
                  <td sx={stylexStyles3.rule39}>{item.rate}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p sx={stylexStyles3.rule40}>
          Most users stay well within the included baseline. No hidden fees.
        </p>
      </section>

      {/* Trust signals */}
      <section sx={stylexStyles3.rule41}>
        <div sx={stylexStyles3.rule42}>
          <span sx={stylexStyles3.rule43}>
            <LuCheck sx={stylexStyles.rule5} />
            Cancel anytime
          </span>
          <span sx={stylexStyles3.rule43}>
            <LuCheck sx={stylexStyles.rule5} />
            No hidden fees
          </span>
          <span sx={stylexStyles3.rule43}>
            <LuCheck sx={stylexStyles.rule5} />
            Open-source
          </span>
        </div>
      </section>

      {/* Divider */}
      <div sx={stylexStyles3.rule44} />

      {/* FAQ */}
      <FaqSection />
    </LegalPageLayout>
  )
}
