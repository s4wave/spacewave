/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useState, type ReactNode } from 'react'
import type { IconType } from '@s4wave/web/images/icons.js'
import {
  LuArrowLeft,
  LuClock3,
  LuCalendarX,
  LuDownload,
  LuRefreshCw,
  LuShield,
  LuTrash2,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  FaqAccordion,
  PageFooter,
  PageWrapper,
} from '@s4wave/app/provider/spacewave/CloudConfirmationPage.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule4: {
    maxWidth: 'var(--container-xl)',
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule8: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule9: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 45%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    textWrap: 'balance',
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 45%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule18: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
  rule19: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule20: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule21: {
    display: 'flex',
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
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule22: {
    display: 'flex',
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 12%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule23: {
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
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
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
  },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule25: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: { color: themeColors['--color-foreground'] },
  rule27: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'inline-flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule28: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule29: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule30: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule31: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    textAlign: 'center',
  },
  rule32: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule33: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule34: {
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
        ':hover': themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule35: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 55%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    color: themeColors['--color-brand'],
  },
})

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
    <div sx={stylexStyles3.rule1}>
      <AnimatedLogo followMouse={false} />
      <div sx={stylexStyles3.rule2}>
        <LuCalendarX sx={stylexStyles.rule1} />
        {cancelBadgeLabel(stage)}
      </div>
      <h1 sx={stylexStyles3.rule3}>{cancelTitle(stage, endLabel)}</h1>
      <p sx={stylexStyles3.rule4}>{cancelSubtitle(stage)}</p>
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
        <span sx={stylexStyles3.rule5}>{endLabel}</span>. After that, your cloud
        data stays read-only for 30 days so you can export what you need.
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
    <div sx={stylexStyles3.rule6}>
      <div sx={stylexStyles3.rule7}>
        <LuCalendarX sx={stylexStyles.rule2} />
      </div>
      <div>
        <h2 sx={[nativeChildSpacing.s2, stylexStyles3.rule8]}>
          {stage === 'active' ? 'If you cancel now' : 'What happens next'}
        </h2>
        <p sx={[nativeChildSpacing.s2, stylexStyles3.rule9]}>
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
    <div sx={stylexStyles3.rule10}>
      <div sx={stylexStyles3.rule11}>
        <div sx={stylexStyles3.rule12}>
          <Icon sx={stylexStyles2.rule1} />
        </div>
        <h3 sx={stylexStyles3.rule13}>{title}</h3>
      </div>
      <p sx={stylexStyles3.rule14}>{children}</p>
    </div>
  )
}

/** CancelOutcomeCards renders the access, export window, and undo cards. */
function CancelOutcomeCards({ stage, endLabel }: CancelHeaderProps) {
  return (
    <div sx={stylexStyles3.rule15}>
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
    <div sx={stylexStyles3.rule16}>
      <div sx={stylexStyles3.rule17}>
        <div sx={stylexStyles3.rule18}>
          <LuTriangleAlert sx={stylexStyles.rule3} />
        </div>
        <div>
          <h3 sx={[nativeChildSpacing.s1, stylexStyles3.rule13]}>
            Need account deletion instead?
          </h3>
          <p sx={[nativeChildSpacing.s1, stylexStyles3.rule9]}>
            Deleting your account requires email verification and works
            differently from standard end-of-period cancellation.
          </p>
          <div sx={[nativeChildSpacing.s1, stylexStyles3.rule19]}>
            <LuTrash2 sx={stylexStyles.rule1} />
            Go to account settings and choose{' '}
            <span sx={stylexStyles3.rule5}>Delete account</span> if that is the
            path you want.
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
    <div sx={stylexStyles3.rule20}>
      {stage === 'scheduled' ? (
        <button
          type="button"
          onClick={onKeep}
          disabled={action !== 'idle' || polling}
          sx={[stylexStyles3.rule21]}
        >
          <LuRefreshCw
            sx={[
              stylexStyles.rule4,
              action === 'reactivating' && stylexStyles.rule5,
            ]}
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
          sx={[stylexStyles3.rule22]}
        >
          <LuCalendarX sx={stylexStyles.rule4} />
          {action === 'canceling' ? 'Canceling…' : 'Cancel at period end'}
        </button>
      )}
      <button type="button" onClick={onBack} sx={stylexStyles3.rule23}>
        {stage === 'scheduled' ? 'Back to billing' : 'Keep my plan'}
      </button>
    </div>
  )
}

/** CancelCheckoutNotice shows the reactivation checkout progress and retry. */
function CancelCheckoutNotice({ checkout }: { checkout: CheckoutState }) {
  return (
    <div sx={stylexStyles3.rule24}>
      <div sx={stylexStyles3.rule25}>
        <LuRefreshCw sx={stylexStyles.rule6} />
        <span sx={stylexStyles3.rule26}>
          Reactivation is in progress. You will return to billing details when
          Stripe confirms the checkout.
        </span>
      </div>
      {checkout.showRetry && (
        <button
          type="button"
          onClick={checkout.continueCheckout}
          sx={stylexStyles3.rule27}
        >
          <LuRefreshCw sx={stylexStyles.rule1} />
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
        <div sx={stylexStyles3.rule28}>
          <LoadingInline
            label="Loading subscription details"
            tone="muted"
            size="sm"
          />
        </div>
      )}
      {!loading && stage === 'grace' && (
        <p sx={stylexStyles3.rule29}>
          This subscription is already canceled. If you want cloud access again,
          you can start a new checkout from the plan page.
        </p>
      )}
      {checkout.polling && <CancelCheckoutNotice checkout={checkout} />}
      {displayError && <p sx={stylexStyles3.rule30}>{displayError}</p>}
    </>
  )
}

/** CancelFaq renders the cancellation questions. */
function CancelFaq() {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div sx={stylexStyles3.rule31}>
        <h2 sx={stylexStyles3.rule32}>Questions before you cancel?</h2>
        <p sx={stylexStyles3.rule33}>The short version, in plain language.</p>
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
        <button type="button" onClick={handleBack} sx={stylexStyles3.rule34}>
          <LuArrowLeft sx={stylexStyles.rule4} />
          Back to billing
        </button>
      }
    >
      {checkout.consentDialog}
      <CancelHeader stage={stage} endLabel={endLabel} />

      <div sx={stylexStyles3.rule35}>
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
