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
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    rotate: '45deg',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
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
    display: 'grid',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
  },
  rule12: { gridTemplateRows: '1fr', opacity: 1 },
  rule13: { gridTemplateRows: '0fr', opacity: 0 },
  rule14: { overflow: 'hidden' },
  rule15: {
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
  },
  rule17: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'block',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule18: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
  },
  rule19: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule20: {
    position: 'relative',
    display: 'flex',
    flexDirection: 'column',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '2px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule21: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * -3)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: 'var(--color-black)',
  },
  rule22: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule23: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'baseline',
    gap: themeLayout['--spacing'],
  },
  rule25: {
    fontSize: 'var(--text-4xl)',
    lineHeight: 'var(--tw-leading, var(--text-4xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    color: themeColors['--color-foreground'],
  },
  rule26: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule27: { flex: 1 },
  rule28: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule29: {
    display: 'flex',
    flexDirection: 'column',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule30: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule31: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'block',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule32: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule33: {
    width: '100%',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule34: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule35: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule36: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'right',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule37: {
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule38: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground'],
  },
  rule39: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule40: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule41: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule42: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule43: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule44: {
    marginInline: 'auto',
    height: '1px',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    cursor: 'pointer',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
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
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
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
