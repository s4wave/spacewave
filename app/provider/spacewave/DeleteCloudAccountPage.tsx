import { useCallback, useEffect, useId, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuBanknote,
  LuCircleCheck,
  LuLogOut,
  LuMail,
  LuRotateCcw,
  LuTimer,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { LogoutConfirmDialog } from '@s4wave/app/session/LogoutConfirmDialog.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import {
  SessionContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { credentialInputStyles } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
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
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginBlock: 'auto',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-lg)',
    width: '100%',
  },
  rule3: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    alignSelf: 'flex-start',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * -2)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    WebkitUserSelect: 'none',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    userSelect: 'none',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
    maxWidth: 'var(--container-sm)',
  },
  rule7: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule11: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule12: { flex: 1 },
  rule13: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule14: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule15: { color: themeColors['--color-foreground'] },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule17: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule18: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    paddingTop: themeLayout['--spacing'],
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule20: {
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule21: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule22: {
    '--tw-tracking': 'var(--tracking-wider)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule23: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-4xl)',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-4xl--line-height))',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule24: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule25: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    marginTop: themeLayout['--spacing'],
  },
  rule26: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule27: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
  rule28: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule29: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule30: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule31: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule32: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

function formatMoney(
  amount: bigint | number | null | undefined,
  currency: string,
) {
  if (amount == null) return null
  const cents = typeof amount === 'bigint' ? Number(amount) : amount
  return `$${(Math.abs(cents) / 100).toFixed(2)} ${currency.toUpperCase()}`
}

function formatDeleteSummary(result: {
  invoiceTotal?: bigint
  invoiceAmountDue?: bigint
  invoiceCurrency?: string
  invoiceStatus?: string
  chargeAttempted?: boolean
  refundAmount?: bigint
  refundCurrency?: string
}) {
  const currency = result.invoiceCurrency || result.refundCurrency || 'usd'
  if (result.refundAmount && result.refundAmount > 0n) {
    return (
      'Refund issued: ' +
      formatMoney(result.refundAmount, result.refundCurrency || currency)
    )
  }
  if (
    result.chargeAttempted &&
    result.invoiceAmountDue &&
    result.invoiceAmountDue > 0n
  ) {
    const amount = formatMoney(result.invoiceAmountDue, currency)
    if (result.invoiceStatus === 'paid') {
      return 'Final charge paid: ' + amount
    }
    return 'Final charge attempted; invoice remains open: ' + amount
  }
  if (result.invoiceTotal && result.invoiceTotal > 0n) {
    return (
      'Outstanding balance recorded: ' +
      formatMoney(result.invoiceTotal, currency)
    )
  }
  if (result.invoiceTotal && result.invoiceTotal < 0n) {
    return (
      'Final credit calculated: ' + formatMoney(result.invoiceTotal, currency)
    )
  }
  return 'Deletion confirmed. The account is now read-only.'
}

function formatCountdown(ms: number): { label: string; sub: string } {
  if (ms <= 0) return { label: 'Finalizing', sub: 'Deletion window has ended' }
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const hh = String(hours).padStart(2, '0')
  const mm = String(minutes).padStart(2, '0')
  const ss = String(seconds).padStart(2, '0')
  return { label: `${hh}:${mm}:${ss}`, sub: 'remaining to undo' }
}

// DeleteCloudAccountPage drives the in-app delete-now email/code flow.
export function DeleteCloudAccountPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const onboarding = SpacewaveOnboardingContext.useContext().onboarding
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const sessionIdx = useSessionIndex()
  const { providerId, accountId, peerId } = useSessionInfo(session)
  const accountResource = useMountAccount(providerId, accountId)
  const account = accountResource.value
  const lifecycleState = onboarding?.lifecycleState
  const deleteAt = onboarding?.deleteAt

  const [sending, setSending] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [undoing, setUndoing] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutOpen, setLogoutOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [retryAfter, setRetryAfter] = useState(0)
  const [code, setCode] = useState('')
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (retryAfter <= 0) return
    const id = setTimeout(() => setRetryAfter((v) => v - 1), 1000)
    return () => clearTimeout(id)
  }, [retryAfter])

  const isPendingDelete =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY

  useEffect(() => {
    if (!isPendingDelete || !deleteAt) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [isPendingDelete, deleteAt])

  const countdown = useMemo(() => {
    if (!isPendingDelete || !deleteAt) return null
    return formatCountdown(Number(deleteAt) - now)
  }, [isPendingDelete, deleteAt, now])

  const deleteAtLabel = useMemo(() => {
    if (!deleteAt) return null
    return new Date(Number(deleteAt)).toLocaleString()
  }, [deleteAt])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleSend = useCallback(async () => {
    if (!session) return
    setSending(true)
    try {
      const resp = await session.spacewave.requestDeleteNowEmail()
      setEmail(resp.email ?? '')
      setRetryAfter(resp.retryAfter ?? 0)
      toast.success('Confirmation email sent')
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : 'Failed to send confirmation email',
      )
    } finally {
      setSending(false)
    }
  }, [session])

  const handleConfirm = useCallback(async () => {
    if (!session || code.length !== 6) return
    setVerifying(true)
    try {
      const resp = await session.spacewave.confirmDeleteNowCode(code)
      toast.success(formatDeleteSummary(resp))
      navigate({ path: '../', replace: true })
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Delete confirmation failed',
      )
    } finally {
      setVerifying(false)
    }
  }, [code, navigate, session])

  const handleUndo = useCallback(async () => {
    if (!session) return
    setUndoing(true)
    try {
      await session.spacewave.undoDeleteNow()
      toast.success(
        'Deletion canceled. The account stays read-only until you resubscribe.',
      )
      navigate({ path: '../', replace: true })
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to cancel deletion',
      )
    } finally {
      setUndoing(false)
    }
  }, [navigate, session])

  const handleLogout = useCallback(async () => {
    if (!account || !peerId || peerId === 'Unknown') return
    setLoggingOut(true)
    await account.selfRevokeSession(peerId).catch(() => {})
    const root = rootResource.value
    if (root && sessionIdx != null) {
      await root.deleteSession(sessionIdx).catch(() => {})
    }
    navigate({ path: '/sessions', replace: true })
  }, [account, navigate, peerId, rootResource.value, sessionIdx])

  return (
    <SessionFrame>
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <button type="button" onClick={handleBack} sx={stylexStyles2.rule3}>
            <LuArrowLeft sx={stylexStyles.rule1} />
            Back
          </button>

          <div sx={stylexStyles2.rule4}>
            <AnimatedLogo followMouse={false} />
            <h1 sx={stylexStyles2.rule5}>
              {isPendingDelete ? 'Deletion Scheduled' : 'Delete Account'}
            </h1>
            <p sx={stylexStyles2.rule6}>
              {isPendingDelete
                ? 'Your cloud account is read-only. You can still undo before the window closes.'
                : "Close your Spacewave Cloud account. You'll have 24 hours to undo it before deletion is final."}
            </p>
          </div>

          {isPendingDelete ? (
            <>
              <PendingDeleteView
                countdown={countdown}
                deleteAtLabel={deleteAtLabel}
                undoing={undoing}
                loggingOut={loggingOut}
                onUndo={() => void handleUndo()}
                onDashboard={handleBack}
                onLogout={() => setLogoutOpen(true)}
              />
              <LogoutConfirmDialog
                open={logoutOpen}
                onOpenChange={setLogoutOpen}
                loggingOut={loggingOut}
                onConfirm={() => void handleLogout()}
              />
            </>
          ) : (
            <InitiateDeleteView
              sending={sending}
              verifying={verifying}
              retryAfter={retryAfter}
              email={email}
              code={code}
              onCodeChange={setCode}
              onSend={() => void handleSend()}
              onConfirm={() => void handleConfirm()}
              onCancel={handleBack}
            />
          )}
        </div>
      </div>
    </SessionFrame>
  )
}

// InitiateDeleteView renders the card stack used before deletion starts.
function InitiateDeleteView({
  sending,
  verifying,
  retryAfter,
  email,
  code,
  onCodeChange,
  onSend,
  onConfirm,
  onCancel,
}: {
  sending: boolean
  verifying: boolean
  retryAfter: number
  email: string
  code: string
  onCodeChange: (v: string) => void
  onSend: () => void
  onConfirm: () => void
  onCancel: () => void
}) {
  const codeValid = code.length === 6
  const canSend = !sending && retryAfter <= 0
  const deleteCodeId = useId()

  return (
    <>
      <div sx={[stylexStyles2.rule7]}>
        <h2 sx={stylexStyles2.rule8}>What happens</h2>
        <ul sx={stylexStyles2.rule9}>
          <InfoRow
            icon={<LuTimer sx={stylexStyles.rule2} />}
            title="Subscription ends now"
            body="Your account becomes read-only. We'll issue the final invoice immediately."
          />
          <InfoRow
            icon={<LuRotateCcw sx={stylexStyles.rule3} />}
            title="24 hours to undo"
            body="Before the window closes, you can cancel deletion from this page or the confirmation email."
          />
          <InfoRow
            icon={<LuBanknote sx={stylexStyles.rule4} />}
            title="Prorated refund if any"
            body="Any unused time is refunded to your card, less Stripe's $0.30 processing fee."
          />
        </ul>
      </div>

      <div sx={[[stylexStyles3.rule1], false]}>
        <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule10]}>
          <div sx={stylexStyles2.rule11}>
            <LuTriangleAlert sx={stylexStyles.rule5} />
          </div>
          <div sx={stylexStyles2.rule12}>
            <h2 sx={stylexStyles2.rule13}>Confirm by email</h2>
            <p sx={stylexStyles2.rule14}>
              {email ? (
                <>
                  We sent a link and 6-digit code to{' '}
                  <strong sx={stylexStyles2.rule15}>{email}</strong>. Click the
                  link or enter the code below.
                </>
              ) : (
                "We'll email you a confirmation link and a 6-digit code. Use either to finalize deletion."
              )}
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={onSend}
          disabled={!canSend}
          sx={[nativeChildSpacingNext.s4, [stylexStyles2.rule16]]}
        >
          {sending ? (
            <Spinner />
          ) : email ? (
            <LuCircleCheck sx={stylexStyles.rule3} />
          ) : (
            <LuMail sx={stylexStyles.rule6} />
          )}
          <span sx={stylexStyles2.rule15}>
            {sending
              ? 'Sending…'
              : retryAfter > 0
                ? `Resend in ${retryAfter}s`
                : email
                  ? 'Resend confirmation email'
                  : 'Send confirmation email'}
          </span>
        </button>

        <div sx={nativeChildSpacingNext.s4}>
          <label
            sx={[nativeChildSpacing.s1_5, stylexStyles2.rule17]}
            htmlFor={deleteCodeId}
          >
            6-digit delete code
          </label>
          <input
            id={deleteCodeId}
            type="text"
            inputMode="numeric"
            maxLength={6}
            value={code}
            onChange={(e) => onCodeChange(e.target.value.replace(/\D/g, ''))}
            placeholder="000000"
            sx={[
              nativeChildSpacing.s1_5,
              [credentialInputStyles.input, credentialInputStyles.code],
            ]}
          />
        </div>

        <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule18]}>
          <button
            type="button"
            onClick={onConfirm}
            disabled={verifying || !codeValid}
            sx={[stylexStyles2.rule19]}
          >
            {verifying && <Spinner />}
            {verifying ? 'Confirming…' : 'Confirm delete account'}
          </button>
          <button type="button" onClick={onCancel} sx={stylexStyles2.rule20}>
            Cancel and return to dashboard
          </button>
        </div>
      </div>
    </>
  )
}

// PendingDeleteView renders the undo-focused state after deletion is scheduled.
function PendingDeleteView({
  countdown,
  deleteAtLabel,
  undoing,
  loggingOut,
  onUndo,
  onDashboard,
  onLogout,
}: {
  countdown: { label: string; sub: string } | null
  deleteAtLabel: string | null
  undoing: boolean
  loggingOut: boolean
  onUndo: () => void
  onDashboard: () => void
  onLogout: () => void
}) {
  return (
    <>
      {countdown && (
        <div sx={[[stylexStyles2.rule7], stylexStyles2.rule21]}>
          <span sx={stylexStyles2.rule22}>Undo window</span>
          <span sx={stylexStyles2.rule23}>{countdown.label}</span>
          <span sx={stylexStyles2.rule24}>{countdown.sub}</span>
          {deleteAtLabel && (
            <span sx={stylexStyles2.rule25}>
              Deletion final at {deleteAtLabel}
            </span>
          )}
        </div>
      )}

      <div sx={[[stylexStyles3.rule1], false]}>
        <button
          type="button"
          onClick={onUndo}
          disabled={undoing}
          sx={[nativeChildSpacingNext.s3, [stylexStyles2.rule26]]}
        >
          {undoing ? <Spinner /> : <LuRotateCcw sx={stylexStyles.rule6} />}
          {undoing ? 'Canceling…' : 'Undo deletion'}
        </button>
        <p sx={[nativeChildSpacingNext.s3, stylexStyles2.rule27]}>
          Undo stops deletion, but the account stays lapsed and read-only until
          you start a new subscription.
        </p>
      </div>

      <div sx={stylexStyles2.rule28}>
        <button type="button" onClick={onDashboard} sx={stylexStyles2.rule29}>
          <LuArrowLeft sx={stylexStyles.rule7} />
          Return to dashboard
        </button>
        <button
          type="button"
          onClick={onLogout}
          disabled={loggingOut}
          sx={stylexStyles2.rule30}
        >
          <LuLogOut sx={stylexStyles.rule7} />
          {loggingOut ? 'Logging out…' : 'Log out of this device'}
        </button>
      </div>
    </>
  )
}

// InfoRow renders a single icon + title + body bullet used in the info card.
function InfoRow({
  icon,
  title,
  body,
}: {
  icon: React.ReactNode
  title: string
  body: string
}) {
  return (
    <li sx={stylexStyles2.rule10}>
      <div sx={stylexStyles2.rule31}>{icon}</div>
      <div sx={stylexStyles2.rule12}>
        <p sx={stylexStyles2.rule32}>{title}</p>
        <p sx={stylexStyles2.rule14}>{body}</p>
      </div>
    </li>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
