import type { IconType } from '@s4wave/web/images/icons.js'
import type { BillingConsent } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { BillingConsentForm } from './useBillingConsent.js'
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
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  PLAN_PRICE_MONTHLY,
  OVERAGE_EXPLANATION,
  STORAGE_BASELINE_GB,
  STORAGE_RATE_DISPLAY,
} from '@s4wave/app/provider/spacewave/pricing.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

import { groupMarker as groupMarker3 } from '../../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule6: {
    padding: `calc(${themeLayout['--spacing']} * 8)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule7: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule9: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule11: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    gap: themeLayout['--spacing'],
    alignItems: 'baseline',
    display: 'flex',
  },
  rule13: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-3xl)',
    fontWeight: 'var(--font-weight-bold)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
  },
  rule14: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule15: { marginTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule16: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule17: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule18: { borderBottomRightRadius: 0, borderTopRightRadius: 0 },
  rule19: {
    '--tw-duration': '.3s',
    borderColor: themeColors['--color-brand'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    borderBottomRightRadius: themeRadii['--radius-md'],
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: 0,
    borderTopRightRadius: themeRadii['--radius-md'],
  },
  rule20: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule21: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule23: {
    '--tw-gradient-from': `color-mix(in oklab, ${themeColors['--color-blue-500']} 5%, transparent)`,
    '--tw-gradient-position': 'to right in oklab',
    '--tw-gradient-stops':
      'var(--tw-gradient-via-stops, var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-to) var(--tw-gradient-to-position))',
    '--tw-gradient-to': `color-mix(in oklab, ${themeColors['--color-cyan-500']} 5%, transparent)`,
    '--tw-gradient-via': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-gradient-via-stops':
      'var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-via) var(--tw-gradient-via-position), var(--tw-gradient-to) var(--tw-gradient-to-position)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundImage: 'linear-gradient(var(--tw-gradient-stops))',
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    justifyContent: 'space-between',
  },
  rule24: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-blue-500']} 10%, transparent)`,
  },
  rule25: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule26: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule27: {
    '--tw-outline-style': 'none',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
  },
  rule28: {
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-2xl)',
    width: '100%',
  },
  rule29: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    marginBlock: 'auto',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-2xl)',
    width: '100%',
  },
  rule30: { paddingTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule31: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule32: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule33: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule34: { display: 'block' },
  rule35: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 70%, transparent)`,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule36: { display: 'block', marginTop: themeLayout['--spacing'] },
  rule37: { marginInline: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule38: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    cursor: 'pointer',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule39: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule40: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule41: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule42: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-leading': 'var(--leading-relaxed)',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--leading-relaxed)',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule43: { color: themeColors['--color-foreground'] },
  rule44: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-foreground'],
      },
    },
  },
  rule45: {
    borderRadius: '.25rem',
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule46: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
    rotate: '45deg',
  },
  rule47: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule48: {
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    display: 'grid',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
  },
  rule49: { gridTemplateRows: '1fr', opacity: 1 },
  rule50: { gridTemplateRows: '0fr', opacity: 0 },
  rule51: { overflow: 'hidden' },
  rule52: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule53: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    '--tw-duration': '.3s',
    '--tw-scale-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    '--tw-scale-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    '--tw-scale-z': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
    transitionDuration: '.3s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    marginRight: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

const CLOUD_EXPANDED_FEATURES = [
  { icon: LuGlobe, text: 'Cloud sync and backup across all devices' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuServer, text: `${STORAGE_BASELINE_GB} GiB cloud storage included` },
  {
    icon: LuZap,
    text: `Extra storage at ${STORAGE_RATE_DISPLAY}`,
  },
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
    sx={stylexStyles4.rule1}
    onClick={(e) => e.stopPropagation()}
  >
    a standard approach to data protection
  </a>
)

const CANCEL_FAQ_ANSWER =
  'Yes. Standard cancellation keeps your subscription active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export what you need or re-subscribe. If you want to fully delete your account, that is handled separately and requires email verification.'

const OVERAGE_FAQ_ANSWER = OVERAGE_EXPLANATION

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
  onRetry: (consent?: BillingConsent) => void
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
        <button type="button" onClick={onBack} sx={stylexStyles4.rule2}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back to plan selection
        </button>
      }
    >
      {/* Header */}
      <div sx={stylexStyles4.rule3}>
        <AnimatedLogo followMouse={false} />
        <h1 sx={stylexStyles4.rule4}>Spacewave Cloud</h1>
        <p sx={stylexStyles4.rule5}>
          Always-on sync, backup, and collaboration
        </p>
      </div>

      {/* Expanded cloud card */}
      <div sx={stylexStyles4.rule6}>
        <div sx={stylexStyles4.rule7}>
          <div sx={stylexStyles4.rule8}>
            <div sx={stylexStyles4.rule9}>
              <LuCloud sx={stylexStyles.rule2} />
            </div>
            <div>
              <h2 sx={stylexStyles4.rule10}>Cloud</h2>
              <p sx={stylexStyles4.rule11}>Everything you need</p>
            </div>
          </div>
          <div sx={stylexStyles4.rule12}>
            <span sx={stylexStyles4.rule13}>${PLAN_PRICE_MONTHLY}</span>
            <span sx={stylexStyles4.rule14}>/ month</span>
          </div>
        </div>

        <FeatureGrid features={CLOUD_EXPANDED_FEATURES} />

        {!checkoutUrl && !polling && (
          <div sx={stylexStyles4.rule15}>
            <BillingConsentForm
              disabled={loading || !root}
              onAccept={onRetry}
            />
          </div>
        )}
        {/* Checkout continuation */}
        {(checkoutUrl || polling) && (
          <div sx={stylexStyles4.rule16}>
            <button
              type="button"
              onClick={() => {
                if (showRetry && checkoutUrl) {
                  window.open(checkoutUrl, '_blank', 'noopener')
                  onLoading?.()
                } else {
                  onRetry()
                }
              }}
              disabled={loading || !root}
              sx={[
                stylexStyles4.rule17,
                showRetry ? stylexStyles4.rule18 : false,
              ]}
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
                type="button"
                onClick={() => onRetry()}
                sx={stylexStyles4.rule19}
                title="Retry"
              >
                <LuRefreshCw sx={stylexStyles.rule1} />
              </button>
            )}
          </div>
        )}

        {error && <p sx={stylexStyles4.rule20}>{error}</p>}
      </div>

      {/* Trust signals */}
      <div sx={stylexStyles4.rule21}>
        {TRUST_SIGNALS.map((text) => (
          <span key={text} sx={stylexStyles4.rule22}>
            <LuCheck sx={stylexStyles.rule3} />
            {text}
          </span>
        ))}
      </div>

      {/* Cloud FAQ */}
      <FaqAccordion items={CLOUD_FAQ} />

      {/* Open source footer */}
      <div sx={stylexStyles4.rule23}>
        <div sx={stylexStyles4.rule8}>
          <div sx={stylexStyles4.rule24}>
            <LuCode sx={stylexStyles.rule2} />
          </div>
          <div>
            <h3 sx={stylexStyles4.rule25}>Open Source Software</h3>
            <p sx={stylexStyles4.rule11}>Built in the open, for everyone</p>
          </div>
        </div>
        <a
          href="https://github.com/aperturerobotics"
          target="_blank"
          rel="noopener noreferrer"
          sx={[stylexStyles3.rule1, groupMarker2]}
        >
          <LuGithub sx={stylexStyles.rule4} />
          <span sx={stylexStyles4.rule26}>View on GitHub</span>
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
    <div sx={stylexStyles4.rule27}>
      {backButton && <div sx={stylexStyles4.rule28}>{backButton}</div>}
      <div sx={[stylexStyles4.rule29, !!backButton && stylexStyles4.rule30]}>
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
    icon: IconType
    text: string
  }[]
}) {
  return (
    <div sx={stylexStyles4.rule31}>
      {features.map((feature) => (
        <div key={feature.text} sx={stylexStyles4.rule32}>
          <feature.icon sx={stylexStyles2.rule1} />
          <span sx={stylexStyles4.rule14}>{feature.text}</span>
        </div>
      ))}
    </div>
  )
}

// PageFooter renders the bottom attribution and legal links.
export function PageFooter() {
  return (
    <p sx={stylexStyles4.rule33}>
      <span sx={stylexStyles4.rule34}>
        Spacewave Cloud by{' '}
        <a
          href="https://aperture.us"
          target="_blank"
          rel="noopener noreferrer"
          sx={stylexStyles4.rule35}
        >
          Aperture Robotics
        </a>
        , LLC. powered by Cloudflare
      </span>
      <span sx={stylexStyles4.rule36}>
        {FOOTER_LINKS.map((link, index) => (
          <span key={link.href}>
            {index > 0 && <span sx={stylexStyles4.rule37}>|</span>}
            <a href={link.href} sx={stylexStyles4.rule35}>
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
        stylexStyles4.rule38,
        isOpen ? stylexStyles4.rule39 : stylexStyles4.rule40,
      ]}
      onClick={onToggle}
      onKeyDown={handleFaqKeyDown}
    >
      <div sx={stylexStyles4.rule41}>
        <h3
          sx={[
            stylexStyles4.rule42,
            isOpen ? stylexStyles4.rule43 : stylexStyles4.rule44,
          ]}
        >
          {question}
        </h3>
        <div
          sx={[
            stylexStyles4.rule45,
            isOpen ? stylexStyles4.rule46 : stylexStyles4.rule47,
          ]}
        >
          <LuPlus sx={stylexStyles.rule5} />
        </div>
      </div>
      <div
        sx={[
          stylexStyles4.rule48,
          isOpen ? stylexStyles4.rule49 : stylexStyles4.rule50,
        ]}
      >
        <div sx={stylexStyles4.rule51}>
          <p sx={stylexStyles4.rule52}>{answer}</p>
        </div>
      </div>
    </div>
  )
}

// FaqAccordion renders a list of FAQ items as an accordion.
export function FaqAccordion({
  sx,
  items,
}: {
  items: { question: string; answer: React.ReactNode }[]
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [openIndex, setOpenIndex] = useState<number>(-1)
  const handleToggle = useCallback((index: number) => {
    setOpenIndex((prev) => (prev === index ? -1 : index))
  }, [])

  return (
    <div sx={[stylexStyles4.rule53, sx]}>
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
