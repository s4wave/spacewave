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
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule5: {
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule10: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    display: 'flex',
    alignItems: 'baseline',
    gap: themeLayout['--spacing'],
  },
  rule13: {
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: { marginTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    gap: themeLayout['--spacing'],
  },
  rule17: {
    display: 'flex',
    flex: 1,
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-brand'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule18: { borderTopRightRadius: 0, borderBottomRightRadius: 0 },
  rule19: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    borderTopRightRadius: themeRadii['--radius-md'],
    borderBottomRightRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: 0,
    borderColor: themeColors['--color-brand'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule20: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule21: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule22: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule23: {
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    '--tw-gradient-position': 'to right in oklab',
    backgroundImage: 'linear-gradient(var(--tw-gradient-stops))',
    '--tw-gradient-from': `color-mix(in oklab, ${themeColors['--color-blue-500']} 5%, transparent)`,
    '--tw-gradient-stops':
      'var(--tw-gradient-via-stops, var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-to) var(--tw-gradient-to-position))',
    '--tw-gradient-via': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-gradient-via-stops':
      'var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-via) var(--tw-gradient-via-position), var(--tw-gradient-to) var(--tw-gradient-to-position)',
    '--tw-gradient-to': `color-mix(in oklab, ${themeColors['--color-cyan-500']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule24: {
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-blue-500']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule25: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule26: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule27: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule28: {
    position: 'relative',
    zIndex: 10,
    width: '100%',
    maxWidth: 'var(--container-2xl)',
  },
  rule29: {
    position: 'relative',
    zIndex: 10,
    marginBlock: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-2xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule30: { paddingTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule31: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule32: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule33: {
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
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
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule36: { marginTop: themeLayout['--spacing'], display: 'block' },
  rule37: { marginInline: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule38: {
    cursor: 'pointer',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule42: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule46: {
    rotate: '45deg',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule47: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule48: {
    display: 'grid',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
  },
  rule49: { gridTemplateRows: '1fr', opacity: 1 },
  rule50: { gridTemplateRows: '0fr', opacity: 0 },
  rule51: { overflow: 'hidden' },
  rule52: {
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule53: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    marginRight: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
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
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
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
