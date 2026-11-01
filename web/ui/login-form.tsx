import React, {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'
import {
  LuFingerprint,
  LuGithub,
  LuKeyRound,
} from '@s4wave/web/images/icons.js'
import { FcGoogle } from '@s4wave/web/images/icons.js'
import { RxArrowRight } from '@s4wave/web/images/icons.js'
import { PiUserCircleDuotone } from '@s4wave/web/ui/phosphor-icons.js'
import { isDesktop } from '@aptre/bldr'
import { useLatestRef } from '@aptre/bldr-react'
import type { CloudProviderConfig } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { AuthProgressCard } from '@s4wave/web/ui/credential/AuthProgressCard.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@s4wave/web/ui/tooltip.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { Turnstile, type TurnstileInstance } from '@s4wave/web/ui/turnstile.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  fonts as themeFonts,
  layout as themeLayout,
  utilityState as themeUtilityState,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { color: themeColors['--color-foreground-alt'] },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    position: 'relative',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    position: 'absolute',
    inset: 0,
    display: 'flex',
    alignItems: 'center',
  },
  rule5: {
    width: '100%',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule6: { position: 'relative', display: 'flex', justifyContent: 'center' },
  rule7: {
    backgroundColor: themeColors['--color-background-get-started'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule9: { color: themeColors['--color-white'] },
  rule10: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule11: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
    opacity: { default: null, ':disabled': 0.5 },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule12: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    borderRadius: '.25rem',
    accentColor: themeColors['--color-brand'],
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    display: 'inline-flex',
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    color: themeColors['--color-brand'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule17: {
    display: 'inline-flex',
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
    textDecorationLine: 'underline',
  },
  rule18: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule19: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule20: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule21: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule22: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule23: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule24: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule25: { color: themeColors['--color-foreground'] },
  rule26: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule27: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },

  rule29: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule30: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule31: { maxWidth: 'var(--container-xs)' },
  rule32: {
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
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
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule33: {
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
  rule34: {
    position: 'relative',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule35: {
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    width: '100%',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
  rule36: {
    display: 'flex',
    height: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule37: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

// dnsLabelRegex validates DNS label format for usernames.
const dnsLabelRegex = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

const inputClassName = cn(
  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50',
  'w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none [@media(pointer:coarse)]:min-h-11',
  'focus:border-brand/50',
  'disabled:opacity-50',
)

// LoginResult represents the outcome of a login attempt.
export type LoginResult =
  | { type: 'session'; sessionIndex: number }
  | { type: 'new_account' }
  | { type: 'error'; errorCode: string }

// LoginMode tracks whether the form is in login or account creation mode.
type LoginMode = 'login' | 'confirm_create'

type BrowserSignInAction = 'browser' | 'passkey' | 'google' | 'github'

// getErrorMessage returns a user-facing message for a login error code.
function getErrorMessage(code: string, method: string): string {
  if (code === 'wrong_password') {
    switch (method) {
      case 'password':
        return 'Wrong password.'
      case 'pem':
        return 'This key is not registered for this account.'
      case 'passkey':
        return 'Passkey not recognized for this account.'
      default:
        return 'Credentials not recognized.'
    }
  }
  return 'Login failed. Please try again.'
}

interface LoginFormProps extends React.ComponentPropsWithoutRef<'div'> {
  initialUsername?: string
  cloudProviderConfig?: CloudProviderConfig | null
  onContinueWithoutAccount?: () => void | Promise<void>
  onLoginWithPassword?: (
    username: string,
    password: string,
    turnstileToken: string,
  ) => Promise<LoginResult>
  onCreateAccountWithPassword?: (
    username: string,
    password: string,
    turnstileToken: string,
  ) => Promise<{ sessionIndex: number }>
  onLoginWithPem?: (pemPrivateKey: Uint8Array) => Promise<{
    sessionIndex: number
  }>
  onNavigateToSession?: (sessionIndex: number, isNew: boolean) => void
  onContinueWithPasskey?: (abortSignal?: AbortSignal) => void | Promise<void>
  onBrowserAuth?: (abortSignal?: AbortSignal) => void | Promise<void>
  onSignInWithSSO?: (
    provider: 'google' | 'github',
    abortSignal?: AbortSignal,
  ) => void | Promise<void>
  onAuthBusyChange?: (busy: boolean) => void
}

const browserSignInPromptDelayMs = 3000

// parseRateLimitError extracts retry_after seconds from an error message.
// Returns 0 if the error is not a rate limit error.
function parseRateLimitError(msg: string): number {
  if (!msg.includes('rate_limited')) return 0
  const match = msg.match(/\[retry_after=(\d+)\]/)
  if (match) return parseInt(match[1], 10)
  return 3
}

// isBrowserAuthRequired checks if the error indicates browser auth is needed.
function isBrowserAuthRequired(msg: string): boolean {
  return msg.includes('browser_auth_required')
}

function stripHost(url: string): string {
  return url.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
}

function buildHashRouteURL(origin: string, path: string): string {
  const route = path.startsWith('/') ? path : `/${path}`
  return origin.replace(/\/+$/, '') + '/#' + route
}

function getBrowserSignInLabel(action: BrowserSignInAction): string {
  switch (action) {
    case 'google':
      return 'Google'
    case 'github':
      return 'GitHub'
    case 'passkey':
      return 'Passkey'
    default:
      return 'browser'
  }
}

function isAbortError(err: unknown): boolean {
  if (err instanceof DOMException && err.name === 'AbortError') return true
  if (!(err instanceof Error)) return false
  const msg = err.message.toLowerCase()
  return (
    msg.includes('abort') ||
    msg.includes('canceled') ||
    msg.includes('cancelled')
  )
}

// SignInMethodButton renders one of the secondary sign-in method buttons
// (PEM backup key, passkey, Google, GitHub). All variants share the same
// outlined surface, hover treatment, and disabled/loading states.
function SignInMethodButton({
  enabled,
  busy,
  loading,
  icon,
  label,
  onClick,
  fullWidth = false,
}: {
  enabled: boolean
  // busy is true when any sign-in is in progress (locks all buttons).
  busy: boolean
  // loading is true when this specific button's action is running.
  loading: boolean
  icon: React.ReactNode
  label: React.ReactNode
  onClick: () => void
  fullWidth?: boolean
}) {
  return (
    <button
      type="button"
      disabled={busy || !enabled}
      onClick={onClick}
      className={
        /* TODO(stylex): global or unknown class: group */ cn(
          'group rounded-md border transition-all duration-300',
          fullWidth ? 'w-full' : 'flex-1',
          'border-foreground/10 bg-background/20',
          enabled
            ? 'hover:border-brand/30 hover:bg-background/40'
            : 'cursor-not-allowed opacity-40',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'flex h-11 items-center justify-center gap-2',
        )
      }
    >
      {loading ? <Spinner size="md" sx={stylexStyles.rule1} /> : icon}
      <span sx={stylexStyles.rule2}>{label}</span>
    </button>
  )
}

// Divider renders a centered label over a horizontal rule.
function Divider({ label }: { label: string }) {
  return (
    <div sx={stylexStyles.rule3}>
      <div sx={stylexStyles.rule4}>
        <div sx={stylexStyles.rule5} />
      </div>
      <div sx={stylexStyles.rule6}>
        <span sx={stylexStyles.rule7}>{label}</span>
      </div>
    </div>
  )
}

// focusNode focuses an input when it mounts.
function focusNode(node: HTMLInputElement | null) {
  node?.focus()
}

type BrowserSignInHandlers = Pick<
  LoginFormProps,
  'onBrowserAuth' | 'onContinueWithPasskey' | 'onSignInWithSSO'
>

// browserSignInRunner picks the handler that runs a browser sign-in action, or
// undefined when the form was not given that handler.
function browserSignInRunner(
  action: BrowserSignInAction,
  {
    onBrowserAuth,
    onContinueWithPasskey,
    onSignInWithSSO,
  }: BrowserSignInHandlers,
): ((abortSignal: AbortSignal) => Promise<void>) | undefined {
  if (action === 'browser') {
    return (
      onBrowserAuth &&
      (async (abortSignal) => {
        await onBrowserAuth(abortSignal)
      })
    )
  }
  if (action === 'passkey') {
    return (
      onContinueWithPasskey &&
      (async (abortSignal) => {
        await onContinueWithPasskey(abortSignal)
      })
    )
  }
  return (
    onSignInWithSSO &&
    (async (abortSignal) => {
      await onSignInWithSSO(action, abortSignal)
    })
  )
}

// authErrorMessage returns the message shown for a sign-in failure.
function authErrorMessage(msg: string, publicHost: string): string {
  if (msg.includes('connection refused')) {
    return 'Cannot reach the server. Please check your connection and try again.'
  }
  if (!isDesktop && msg.includes('unsupported host')) {
    return `Spacewave Cloud sign-in is only supported on ${publicHost}`
  }
  return msg
}

interface PasswordFormState {
  usernameValid: boolean
  passwordValid: boolean
  creatingAccount: boolean
  agreed: boolean
  turnstileReady: boolean
  passwordsMatch: boolean
}

// passwordFormError returns why the password form cannot be submitted, or null
// when it can.
function passwordFormError(form: PasswordFormState): string | null {
  // Validate the current password form before sending an authentication request.
  if (!form.usernameValid) {
    return 'Username must be a valid DNS label (lowercase letters, numbers, hyphens)'
  }
  if (!form.passwordValid) return 'Password must be at least 8 characters'
  if (form.creatingAccount && !form.agreed) {
    return 'You must agree to the Terms of Service and Privacy Policy'
  }
  if (!form.turnstileReady) return 'Loading server configuration'
  if (form.creatingAccount && !form.passwordsMatch) {
    return 'Passwords do not match'
  }
  return null
}

// passwordSubmitBlocked reports whether the password submit button is disabled.
function passwordSubmitBlocked(
  form: PasswordFormState,
  busy: boolean,
  rateLimitCountdown: number,
): boolean {
  return (
    busy ||
    !form.usernameValid ||
    !form.passwordValid ||
    (form.creatingAccount && !form.agreed) ||
    !form.turnstileReady ||
    rateLimitCountdown > 0
  )
}

// LoginFooter renders the tagline below the form.
function LoginFooter({ creatingAccount }: { creatingAccount: boolean }) {
  return (
    <div sx={stylexStyles.rule8}>
      {creatingAccount ? (
        'creating a new cloud account'
      ) : (
        <>
          local-first, end-to-end encrypted,{' '}
          <span sx={stylexStyles.rule9}>no account required</span>
        </>
      )}
    </div>
  )
}

// useRateLimitCountdown counts down a server rate limit and remembers that the
// password submit should be retried once the countdown ends.
function useRateLimitCountdown() {
  // Keep the displayed retry countdown, timer and pending retry together.
  const [countdown, setCountdown] = useState(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingRetryRef = useRef(false)

  // Release the countdown timer when its owning form unmounts.
  useEffect(() => {
    return () => clearTimeout(timerRef.current ?? undefined)
  }, [])

  // Start a countdown for the server's retry delay.
  const start = useCallback((seconds: number) => {
    // Retain one pending retry while decrementing the displayed delay.
    setCountdown(seconds)
    pendingRetryRef.current = true
    let remaining = seconds
    const tick = () => {
      remaining -= 1
      if (remaining <= 0) {
        timerRef.current = null
        setCountdown(0)
        return
      }
      timerRef.current = setTimeout(tick, 1000)
      setCountdown(remaining)
    }
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(tick, 1000)
  }, [])

  // consumeRetry reports whether a retry is pending and clears the pending flag.
  const consumeRetry = useCallback(() => {
    const pending = pendingRetryRef.current
    pendingRetryRef.current = false
    return pending
  }, [])

  return { countdown, start, consumeRetry }
}

interface BrowserSignInOptions extends BrowserSignInHandlers {
  setLoading: (value: string | null) => void
  setError: (error: string | null) => void
  onError: (err: unknown) => void
}

// useBrowserSignIn runs desktop sign-in actions that continue in the system
// browser. After a delay it prompts the user to reopen or cancel the attempt.
function useBrowserSignIn({
  setLoading,
  setError,
  onError,
  ...handlers
}: BrowserSignInOptions) {
  // Keep the active browser sign-in prompt and cancellation controls together.
  const [prompt, setPrompt] = useState<BrowserSignInAction | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  // Clear both the delayed prompt and its visible state.
  const clearPrompt = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    setPrompt(null)
  }, [])

  // Cancel the current browser authentication and release its busy state.
  const cancel = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    clearPrompt()
    setLoading(null)
  }, [clearPrompt, setLoading])

  // Abort browser authentication and clear its prompt timer on unmount.
  useEffect(() => {
    return () => {
      abortRef.current?.abort()
      clearTimeout(timerRef.current ?? undefined)
    }
  }, [])

  // Choose the configured runner for the requested browser sign-in method.
  const { onBrowserAuth, onContinueWithPasskey, onSignInWithSSO } = handlers
  const start = useCallback(
    async (action: BrowserSignInAction) => {
      // Require an available authentication runner before starting its request.
      const runner = browserSignInRunner(action, {
        onBrowserAuth,
        onContinueWithPasskey,
        onSignInWithSSO,
      })
      if (!runner) return

      // Replace the previous request and show its delayed browser prompt.
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      clearPrompt()
      setLoading(action)
      setError(null)
      timerRef.current = setTimeout(() => {
        if (abortRef.current !== controller) return
        if (controller.signal.aborted) return
        setPrompt(action)
      }, browserSignInPromptDelayMs)

      // Complete the selected authentication while ignoring cancellation feedback.
      try {
        await runner(controller.signal)
      } catch (err) {
        if (controller.signal.aborted || isAbortError(err)) return
        onError(err)
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null
          clearPrompt()
          setLoading(null)
        }
      }
    },
    [
      clearPrompt,
      onBrowserAuth,
      onContinueWithPasskey,
      onError,
      onSignInWithSSO,
      setError,
      setLoading,
    ],
  )

  return { prompt, start, cancel }
}

interface SignInActionsOptions extends BrowserSignInHandlers {
  setLoading: (value: string | null) => void
  setError: (error: string | null) => void
  onError: (err: unknown) => void
  startBrowserSignIn: (action: BrowserSignInAction) => Promise<void>
  onContinueWithoutAccount: LoginFormProps['onContinueWithoutAccount']
  onLoginWithPem: LoginFormProps['onLoginWithPem']
  onNavigateToSession: LoginFormProps['onNavigateToSession']
}

// useSignInActions binds the sign-in methods other than the password: backup
// key, passkey, SSO, browser auth, and the local account. Desktop routes the
// browser methods through the browser sign-in prompt.
function useSignInActions({
  setLoading,
  setError,
  onError,
  startBrowserSignIn,
  onContinueWithoutAccount,
  onLoginWithPem,
  onNavigateToSession,
  onContinueWithPasskey,
  onBrowserAuth,
  onSignInWithSSO,
}: SignInActionsOptions) {
  // Keep the chosen backup-key filename with the form's sign-in actions.
  const [pemFileName, setPemFileName] = useState<string | null>(null)

  // Run an available sign-in action with shared busy and error feedback.
  const runAction = useCallback(
    async (action: string, handler?: () => void | Promise<void>) => {
      if (!handler) return
      setLoading(action)
      setError(null)
      try {
        await handler()
      } catch (err) {
        setError(errorText(err))
      } finally {
        setLoading(null)
      }
    },
    [setError, setLoading],
  )

  // Read the selected backup key through the existing file handler.
  const changePemFile = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file) return
      readBackupKey(
        file,
        (key) => {
          setPemFileName(file.name)
          void runAction('pem', async () => {
            const login = await onLoginWithPem?.(key)
            if (!login) return
            onNavigateToSession?.(login.sessionIndex, false)
          })
        },
        () => setError('Failed to read backup key'),
      )
      e.target.value = ''
    },
    [runAction, onLoginWithPem, onNavigateToSession, setError],
  )

  // Run the selected SSO provider with the form's shared request feedback.
  const signInWithSSO = useCallback(
    async (provider: 'google' | 'github') => {
      if (!onSignInWithSSO) return
      setLoading(provider)
      setError(null)
      try {
        await onSignInWithSSO(provider)
      } catch (err) {
        onError(err)
      } finally {
        setLoading(null)
      }
    },
    [onError, onSignInWithSSO, setError, setLoading],
  )

  return {
    pemFileName,
    changePemFile,
    signInWithSSO: (provider: 'google' | 'github') =>
      void signInWithSSO(provider),
    signInWithBrowser: () =>
      isDesktop ? void startBrowserSignIn('browser') : void onBrowserAuth?.(),
    signInWithPasskey: () =>
      isDesktop
        ? void startBrowserSignIn('passkey')
        : void runAction('passkey', onContinueWithPasskey),
    continueWithoutAccount: () =>
      void runAction('continue', onContinueWithoutAccount),
  }
}

// LoginFields renders the username, password, and confirmation inputs.
function LoginFields({
  username,
  password,
  confirm,
  busy,
  creatingAccount,
  passwordsMatch,
  onUsernameChange,
  onPasswordChange,
  onConfirmChange,
}: {
  username: string
  password: string
  confirm: string
  busy: boolean
  creatingAccount: boolean
  passwordsMatch: boolean
  onUsernameChange: (value: string) => void
  onPasswordChange: (value: string) => void
  onConfirmChange: (value: string) => void
}) {
  const usernameId = useId()
  const passwordId = useId()
  const confirmId = useId()

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div>
        <label sx={stylexStyles.rule10} htmlFor={usernameId}>
          Username
        </label>
        <input
          ref={focusNode}
          id={usernameId}
          type="text"
          value={username}
          onChange={(e) => onUsernameChange(e.target.value)}
          placeholder="alice"
          disabled={busy || creatingAccount}
          sx={[stylexStyles.rule11]}
        />
      </div>

      <div>
        <label sx={stylexStyles.rule10} htmlFor={passwordId}>
          Password
        </label>
        <input
          id={passwordId}
          type="password"
          value={password}
          onChange={(e) => onPasswordChange(e.target.value)}
          placeholder="Enter password"
          disabled={busy}
          sx={[stylexStyles.rule11]}
        />
      </div>

      {creatingAccount && (
        <div>
          <label sx={stylexStyles.rule10} htmlFor={confirmId}>
            Confirm password
          </label>
          <input
            ref={focusNode}
            id={confirmId}
            type="password"
            value={confirm}
            onChange={(e) => onConfirmChange(e.target.value)}
            placeholder="Confirm password"
            disabled={busy}
            className={
              /* TODO(stylex): conditional property composition: borderColor */ cn(
                inputClassName,
                confirm.length > 0 &&
                  !passwordsMatch &&
                  'border-destructive/50',
              )
            }
          />
        </div>
      )}

      {password.length > 0 && <PasswordStrength password={password} />}
    </div>
  )
}

// TermsAgreement renders the terms of service checkbox of account creation.
function TermsAgreement({
  agreed,
  busy,
  onChange,
}: {
  agreed: boolean
  busy: boolean
  onChange: (agreed: boolean) => void
}) {
  return (
    <label sx={stylexStyles.rule12}>
      <input
        type="checkbox"
        checked={agreed}
        onChange={(e) => onChange(e.target.checked)}
        disabled={busy}
        sx={stylexStyles.rule13}
      />
      <span sx={stylexStyles.rule14}>
        I agree to the{' '}
        <a
          href="#/tos"
          sx={stylexStyles.rule15}
          onClick={(e) => e.stopPropagation()}
        >
          Terms of Service
        </a>{' '}
        and{' '}
        <a
          href="#/privacy"
          sx={stylexStyles.rule15}
          onClick={(e) => e.stopPropagation()}
        >
          Privacy Policy
        </a>
      </span>
    </label>
  )
}

// LoginNotices renders the error, rate limit, and browser auth notices.
function LoginNotices({
  error,
  forgotPasswordUrl,
  rateLimitCountdown,
  browserAuthRequired,
  onBrowserAuth,
}: {
  error: string | null
  forgotPasswordUrl: string
  rateLimitCountdown: number
  browserAuthRequired: boolean
  onBrowserAuth: () => void
}) {
  return (
    <>
      {error && (
        <div>
          <p sx={stylexStyles.rule16}>{error}</p>
          {error.includes('Wrong password') && (
            <a
              href={forgotPasswordUrl}
              target="_blank"
              rel="noopener noreferrer"
              sx={stylexStyles.rule17}
            >
              Forgot your password?
            </a>
          )}
        </div>
      )}

      {rateLimitCountdown > 0 && (
        <div sx={stylexStyles.rule18}>
          <p sx={stylexStyles.rule19}>
            Rate limited, retrying in {rateLimitCountdown}s…
          </p>
        </div>
      )}

      {browserAuthRequired && (
        <div sx={stylexStyles.rule20}>
          <p sx={stylexStyles.rule21}>
            Enhanced security required. Sign in via your browser.
          </p>
          <button
            type="button"
            onClick={onBrowserAuth}
            sx={[stylexStyles.rule22]}
          >
            <LuKeyRound {...stylex.props(stylexStyles.rule23)} />
            <span sx={stylexStyles.rule24}>Open browser to sign in…</span>
          </button>
        </div>
      )}
    </>
  )
}

const creatingSteps = [
  'Creating secure account keys',
  'Protecting your account credentials',
  'Opening your first session',
]

const signingInSteps = [
  'Checking your credentials',
  'Unlocking your account key',
  'Opening your session',
]

// PasswordSubmit renders the password submit button and, while it runs, the
// progress card.
function PasswordSubmit({
  busy,
  disabled,
  creatingAccount,
  onSubmit,
}: {
  busy: boolean
  disabled: boolean
  creatingAccount: boolean
  onSubmit: () => void
}) {
  let label = 'Continue with password'
  if (creatingAccount) label = 'Confirm and create account'
  if (busy) label = 'Connecting…'

  return (
    <>
      <button
        type="button"
        onClick={onSubmit}
        disabled={disabled}
        className={
          /* TODO(stylex): global or unknown class: group */ cn(
            'group w-full rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:border-foreground/10 disabled:bg-foreground/5 disabled:opacity-60 disabled:hover:bg-foreground/5',
            'flex h-11 items-center justify-center gap-2',
          )
        }
      >
        {busy ? (
          <Spinner sx={stylexStyles.rule25} />
        ) : (
          <LuKeyRound {...stylex.props(stylexStyles.rule23)} />
        )}
        <span sx={stylexStyles.rule24}>{label}</span>
      </button>

      {busy && (
        <AuthProgressCard
          title={
            creatingAccount
              ? 'Creating your secure account'
              : 'Signing in securely'
          }
          detail="Spacewave is securing your account on this device. This can take a moment."
          steps={creatingAccount ? creatingSteps : signingInSteps}
        />
      )}
    </>
  )
}

const ssoProviders = {
  google: {
    label: 'Google',
    icon: <FcGoogle {...stylex.props(stylexStyles.rule26)} />,
  },
  github: {
    label: 'GitHub',
    icon: <LuGithub {...stylex.props(stylexStyles.rule27)} />,
  },
}

// SignInMethods renders the backup key, passkey, and SSO sign-in buttons.
function SignInMethods({
  loading,
  pemFileName,
  canUsePem,
  canUsePasskey,
  ssoEnabled,
  onPemFileChange,
  onPasskey,
  onSSO,
}: {
  loading: string | null
  pemFileName: string | null
  canUsePem: boolean
  canUsePasskey: boolean
  ssoEnabled: Record<'google' | 'github', boolean>
  onPemFileChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  onPasskey: () => void
  onSSO: (provider: 'google' | 'github') => void
}) {
  // Resolve the backup-key control's label and current busy state.
  const pemInputRef = useRef<HTMLInputElement>(null)
  const busy = loading !== null
  let pemLabel = 'Backup key (.pem)'
  if (pemFileName) pemLabel = `Backup key: ${pemFileName}`
  if (loading === 'pem') pemLabel = 'Signing in with backup key...'

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
      }
    >
      <input
        ref={pemInputRef}
        type="file"
        accept=".pem"
        onChange={onPemFileChange}
        hidden
      />
      <SignInMethodButton
        fullWidth
        enabled={canUsePem}
        busy={busy}
        loading={loading === 'pem'}
        icon={<LuKeyRound {...stylex.props(stylexStyles.rule27)} />}
        label={pemLabel}
        onClick={() => pemInputRef.current?.click()}
      />
      <div sx={stylexStyles.rule29}>
        <Tooltip>
          <TooltipTrigger asChild>
            <SignInMethodButton
              enabled={canUsePasskey}
              busy={busy}
              loading={loading === 'passkey'}
              icon={<LuFingerprint {...stylex.props(stylexStyles.rule27)} />}
              label="Passkey"
              onClick={onPasskey}
            />
          </TooltipTrigger>
          {!canUsePasskey && (
            <TooltipContent side="bottom">Coming soon</TooltipContent>
          )}
        </Tooltip>
        {(['google', 'github'] as const).map(
          (provider) =>
            ssoEnabled[provider] && (
              <Tooltip key={provider}>
                <TooltipTrigger asChild>
                  <SignInMethodButton
                    enabled
                    busy={busy}
                    loading={loading === provider}
                    icon={ssoProviders[provider].icon}
                    label={ssoProviders[provider].label}
                    onClick={() => onSSO(provider)}
                  />
                </TooltipTrigger>
              </Tooltip>
            ),
        )}
      </div>
    </div>
  )
}

// ContinueWithoutAccount renders the local account button.
function ContinueWithoutAccount({
  loading,
  onContinue,
}: {
  loading: string | null
  onContinue: () => void
}) {
  return (
    <>
      <Divider label="or" />
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={onContinue}
            disabled={loading !== null}
            className={
              /* TODO(stylex): global or unknown class: group */ cn(
                'group relative w-full overflow-hidden rounded-md border transition-all duration-300',
                'border-foreground/20 bg-background/20 hover:border-brand/30 hover:bg-background/40',
                'disabled:cursor-not-allowed disabled:opacity-50',
                'flex h-11 items-center justify-between px-4',
              )
            }
          >
            <div sx={stylexStyles.rule30}>
              <PiUserCircleDuotone
                className={
                  /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:text-brand */ 'text-foreground-alt group-hover:text-brand size-5 transition-colors'
                }
              />
              <span
                className={
                  /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:text-foreground */ 'text-foreground-alt group-hover:text-foreground text-sm transition-colors'
                }
              >
                {loading === 'continue'
                  ? 'Starting…'
                  : 'Continue without account'}
              </span>
            </div>
            <RxArrowRight
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:translate-x-1 */ cn(
                  'text-foreground-alt group-hover:text-brand size-4 transition-all duration-300',
                  'group-hover:translate-x-1',
                )
              }
            />
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom" sx={stylexStyles.rule31}>
          Creates a local account stored only on your device
        </TooltipContent>
      </Tooltip>
    </>
  )
}

// BrowserSignInDialog asks the user to reopen or cancel a sign-in that
// continues in the system browser.
function BrowserSignInDialog({
  prompt,
  onOpenAgain,
  onCancel,
}: {
  prompt: BrowserSignInAction | null
  onOpenAgain: (action: BrowserSignInAction) => void
  onCancel: () => void
}) {
  return (
    <Dialog
      open={prompt !== null}
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Continue sign-in in your browser</DialogTitle>
          <DialogDescription>
            Spacewave opened a {getBrowserSignInLabel(prompt ?? 'browser')}{' '}
            sign-in page in your web browser. If it did not appear, open it
            again or cancel this attempt.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={() => {
              if (prompt) onOpenAgain(prompt)
            }}
            sx={[stylexStyles.rule32]}
          >
            Open again
          </button>
          <button type="button" onClick={onCancel} sx={[stylexStyles.rule33]}>
            Cancel attempt
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// applyLoginResult routes the outcome of a password login to the form.
function applyLoginResult(
  result: LoginResult,
  actions: {
    navigate: (sessionIndex: number) => void
    confirmCreate: () => void
    fail: (message: string) => void
  },
) {
  switch (result.type) {
    case 'session':
      actions.navigate(result.sessionIndex)
      break
    case 'new_account':
      actions.confirmCreate()
      break
    case 'error':
      actions.fail(getErrorMessage(result.errorCode, 'password'))
      break
  }
}

// submitPassword creates the account or logs in, and routes the outcome.
async function submitPassword(
  creatingAccount: boolean,
  credentials: { username: string; password: string; token: string },
  handlers: Pick<
    LoginFormProps,
    | 'onLoginWithPassword'
    | 'onCreateAccountWithPassword'
    | 'onNavigateToSession'
  >,
  actions: { confirmCreate: () => void; fail: (message: string) => void },
) {
  // Create the account or submit the existing account's password credentials.
  const { username, password, token } = credentials
  const {
    onLoginWithPassword,
    onCreateAccountWithPassword,
    onNavigateToSession,
  } = handlers
  if (creatingAccount) {
    const result = await onCreateAccountWithPassword?.(
      username,
      password,
      token,
    )
    if (result) onNavigateToSession?.(result.sessionIndex, true)
    return
  }

  // Apply the login result through the configured navigation and recovery actions.
  const result = await onLoginWithPassword?.(username, password, token)
  if (result) {
    applyLoginResult(result, {
      navigate: (sessionIndex) => onNavigateToSession?.(sessionIndex, false),
      ...actions,
    })
  }
}

// errorText returns the message of a thrown value.
function errorText(err: unknown): string {
  return err instanceof Error ? err.message : 'An error occurred'
}

// loginConfig derives the cloud settings that the form depends on.
function loginConfig(
  config: CloudProviderConfig | null | undefined,
  canSignInWithSSO: boolean,
) {
  const publicBaseUrl = config?.publicBaseUrl ?? SPACEWAVE_PUBLIC_BASE_URL
  const turnstileSiteKey = config?.turnstileSiteKey ?? ''

  return {
    turnstileSiteKey,
    turnstileReady: isDesktop || turnstileSiteKey !== '',
    publicHost: stripHost(publicBaseUrl),
    forgotPasswordUrl: buildHashRouteURL(publicBaseUrl, '/recover'),
    ssoEnabled: {
      google: canSignInWithSSO && !!config?.googleSsoEnabled,
      github: canSignInWithSSO && !!config?.githubSsoEnabled,
    },
  }
}

// readBackupKey reads a backup key file, reporting its bytes or a read failure.
function readBackupKey(
  file: File,
  onRead: (key: Uint8Array) => void,
  onFail: () => void,
) {
  const reader = new FileReader()
  reader.onload = () => {
    if (reader.result instanceof ArrayBuffer) {
      onRead(new Uint8Array(reader.result))
    }
  }
  reader.onerror = onFail
  reader.readAsArrayBuffer(file)
}

// SignInAlternatives groups the non-password methods available to this form.
function SignInAlternatives({
  visible,
  loading,
  actions,
  canUsePem,
  canUsePasskey,
  ssoEnabled,
  canContinueWithoutAccount,
}: {
  visible: boolean
  loading: string | null
  actions: ReturnType<typeof useSignInActions>
  canUsePem: boolean
  canUsePasskey: boolean
  ssoEnabled: ReturnType<typeof loginConfig>['ssoEnabled']
  canContinueWithoutAccount: boolean
}) {
  if (!visible) return null

  return (
    <>
      <Divider label="or sign in with" />
      <SignInMethods
        loading={loading}
        pemFileName={actions.pemFileName}
        canUsePem={canUsePem}
        canUsePasskey={canUsePasskey}
        ssoEnabled={ssoEnabled}
        onPemFileChange={actions.changePemFile}
        onPasskey={actions.signInWithPasskey}
        onSSO={actions.signInWithSSO}
      />
      {canContinueWithoutAccount && (
        <ContinueWithoutAccount
          loading={loading}
          onContinue={actions.continueWithoutAccount}
        />
      )}
    </>
  )
}

// useAuthBusyState keeps the owner's busy indication tied to the form's lifetime.
function useAuthBusyState(
  onAuthBusyChange: LoginFormProps['onAuthBusyChange'],
) {
  // Track the active authentication method for this mounted form.
  const [loading, setLoadingState] = useState<string | null>(null)

  // Notify the owner whenever authentication enters or leaves a busy state.
  const onAuthBusyChangeRef = useLatestRef(onAuthBusyChange)
  const setLoading = useCallback(
    (value: string | null) => {
      setLoadingState(value)
      onAuthBusyChangeRef.current?.(value !== null)
    },
    [onAuthBusyChangeRef],
  )

  // Use the latest owner callback when this form becomes idle on unmount.
  const notifyAuthIdleOnUnmount = useEffectEvent(() => {
    onAuthBusyChange?.(false)
  })

  // Release the owner's busy indication when this form unmounts.
  useEffect(() => {
    return () => notifyAuthIdleOnUnmount()
  }, [])

  return [loading, setLoading] as const
}

// LoginForm renders a unified authentication screen with username+password
// fields and a "Continue with password" button that handles both new account
// creation and login to existing accounts.
export function LoginForm({
  className,
  initialUsername,
  cloudProviderConfig,
  onContinueWithoutAccount,
  onLoginWithPassword,
  onCreateAccountWithPassword,
  onLoginWithPem,
  onNavigateToSession,
  onContinueWithPasskey,
  onBrowserAuth,
  onSignInWithSSO,
  onAuthBusyChange,
  ...props
}: LoginFormProps) {
  // Keep the current sign-in draft, mode and request feedback together.
  const [loading, setLoading] = useAuthBusyState(onAuthBusyChange)
  const [username, setUsername] = useState(
    () => initialUsername?.toLowerCase() ?? '',
  )
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<LoginMode>('login')
  const [agreed, setAgreed] = useState(false)
  const [browserAuthRequired, setBrowserAuthRequired] = useState(false)

  // Resolve server verification and the controls for the current authentication mode.
  const turnstileRef = useRef<TurnstileInstance>(null)
  const {
    turnstileSiteKey,
    turnstileReady,
    publicHost,
    forgotPasswordUrl,
    ssoEnabled,
  } = loginConfig(cloudProviderConfig, !!onSignInWithSSO)
  const creatingAccount = mode === 'confirm_create'
  const busy = loading !== null
  const passwordBusy = loading === 'password'

  // Derive field eligibility from the current username and password draft.
  const usernameValid = dnsLabelRegex.test(username)
  const passwordValid = password.length >= 8
  const passwordsMatch = password === confirm

  // Share the retry countdown with the form's authentication error handling.
  const rateLimit = useRateLimitCountdown()

  // Require browser verification while allowing native desktop authentication.
  const getTurnstileToken = useCallback(async (): Promise<string> => {
    if (isDesktop) return ''
    const token = (await turnstileRef.current?.takeToken()) ?? ''
    if (!token) throw new Error('Turnstile verification failed')
    return token
  }, [])

  // Translate authentication failures into browser sign-in, retry delay or visible error state.
  const startRateLimit = rateLimit.start
  const handleAuthError = useCallback(
    (err: unknown) => {
      // Distinguish browser-required and rate-limited authentication from ordinary failures.
      const msg = errorText(err)
      if (isBrowserAuthRequired(msg)) {
        setBrowserAuthRequired(true)
        setError(null)
        return
      }
      const retryAfter = parseRateLimitError(msg)
      if (retryAfter > 0) {
        setBrowserAuthRequired(false)
        setError(null)
        startRateLimit(retryAfter)
        return
      }
      setError(authErrorMessage(msg, publicHost))
    },
    [publicHost, startRateLimit],
  )

  // Bind browser sign-in to the form's current loading and error controls.
  const browserSignIn = useBrowserSignIn({
    setLoading,
    setError,
    onError: handleAuthError,
    onBrowserAuth,
    onContinueWithPasskey,
    onSignInWithSSO,
  })
  const actions = useSignInActions({
    setLoading,
    setError,
    onError: handleAuthError,
    startBrowserSignIn: browserSignIn.start,
    onContinueWithoutAccount,
    onLoginWithPem,
    onNavigateToSession,
    onContinueWithPasskey,
    onBrowserAuth,
    onSignInWithSSO,
  })

  // Keep password submission eligibility in one derived form snapshot.
  const passwordForm = useMemo<PasswordFormState>(
    () => ({
      usernameValid,
      passwordValid,
      creatingAccount,
      agreed,
      turnstileReady,
      passwordsMatch,
    }),
    [
      usernameValid,
      passwordValid,
      creatingAccount,
      agreed,
      turnstileReady,
      passwordsMatch,
    ],
  )
  const handleContinueWithPassword = useCallback(async () => {
    // Validate the current password form before starting its request.
    const invalid = passwordFormError(passwordForm)
    if (invalid) {
      setError(invalid)
      return
    }

    // Acquire verification and submit password authentication with shared request feedback.
    setError(null)
    setLoading('password')
    try {
      const token = await getTurnstileToken()
      await submitPassword(
        creatingAccount,
        { username, password, token },
        {
          onLoginWithPassword,
          onCreateAccountWithPassword,
          onNavigateToSession,
        },
        { confirmCreate: () => setMode('confirm_create'), fail: setError },
      )
    } catch (err) {
      handleAuthError(err)
    } finally {
      setLoading(null)
    }
  }, [
    passwordForm,
    creatingAccount,
    getTurnstileToken,
    handleAuthError,
    onLoginWithPassword,
    onCreateAccountWithPassword,
    onNavigateToSession,
    username,
    password,
    setLoading,
  ])

  // Submit the current password form when Enter is pressed while idle.
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' && loading === null) {
        void handleContinueWithPassword()
      }
    },
    [handleContinueWithPassword, loading],
  )
  const continueWithPassword = useEffectEvent(() => {
    void handleContinueWithPassword()
  })

  // Retry the password submit once the rate limit ends.
  const { countdown: rateLimitCountdown, consumeRetry } = rateLimit
  useEffect(() => {
    if (rateLimitCountdown !== 0 || loading !== null || !consumeRetry()) return
    const id = setTimeout(continueWithPassword, 0)
    return () => clearTimeout(id)
  }, [loading, rateLimitCountdown, consumeRetry])

  // Clear stale errors when the draft changes and reset confirmation on return to login.
  const edit = useCallback((set: (value: string) => void, value: string) => {
    set(value)
    setError(null)
  }, [])
  const returnToLogin = () => {
    setMode('login')
    edit(setConfirm, '')
  }

  return (
    <div
      role="group"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'flex flex-col gap-4',
          className,
        )
      }
      onKeyDown={handleKeyDown}
      {...props}
    >
      <div sx={stylexStyles.rule34}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3 p-6'
          }
        >
          <LoginFields
            username={username}
            password={password}
            confirm={confirm}
            busy={busy}
            creatingAccount={creatingAccount}
            passwordsMatch={passwordsMatch}
            onUsernameChange={(value) => edit(setUsername, value.toLowerCase())}
            onPasswordChange={(value) => edit(setPassword, value)}
            onConfirmChange={(value) => edit(setConfirm, value)}
          />

          {creatingAccount && (
            <TermsAgreement agreed={agreed} busy={busy} onChange={setAgreed} />
          )}

          <LoginNotices
            error={error}
            forgotPasswordUrl={forgotPasswordUrl}
            rateLimitCountdown={rateLimitCountdown}
            browserAuthRequired={browserAuthRequired}
            onBrowserAuth={actions.signInWithBrowser}
          />

          {!isDesktop && turnstileSiteKey !== '' && (
            <Turnstile ref={turnstileRef} siteKey={turnstileSiteKey} />
          )}

          <PasswordSubmit
            busy={passwordBusy}
            disabled={passwordSubmitBlocked(
              passwordForm,
              busy,
              rateLimitCountdown,
            )}
            creatingAccount={creatingAccount}
            onSubmit={() => void handleContinueWithPassword()}
          />

          {!passwordBusy && creatingAccount && (
            <button
              type="button"
              onClick={returnToLogin}
              sx={stylexStyles.rule35}
            >
              &larr; Return to login
            </button>
          )}

          <SignInAlternatives
            visible={!passwordBusy && mode === 'login'}
            loading={loading}
            actions={actions}
            canUsePem={!!onLoginWithPem}
            canUsePasskey={!!onContinueWithPasskey}
            ssoEnabled={ssoEnabled}
            canContinueWithoutAccount={!!onContinueWithoutAccount}
          />
        </div>
      </div>

      <BrowserSignInDialog
        prompt={browserSignIn.prompt}
        onOpenAgain={(action) => void browserSignIn.start(action)}
        onCancel={browserSignIn.cancel}
      />

      <LoginFooter creatingAccount={creatingAccount} />
    </div>
  )
}

function PasswordStrength({ password }: { password: string }) {
  // Resolve the current password's strength label and indicator color.
  const strength =
    password.length === 0
      ? 0
      : password.length < 8
        ? 1
        : password.length < 12
          ? 2
          : 3
  const labels = ['', 'Weak', 'Fair', 'Strong']
  const colors = ['', 'bg-destructive', 'bg-yellow-500', 'bg-green-500']

  // Hide the strength indicator until the person enters a password.
  if (password.length === 0) return null

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1 */ 'space-y-1'
      }
    >
      <div sx={stylexStyles.rule36}>
        {[1, 2, 3].map((level) => (
          <div
            key={level}
            className={
              /* TODO(stylex): dynamic class or forwarded className */ cn(
                'h-full flex-1 rounded-full transition-colors',
                level <= strength ? colors[strength] : 'bg-transparent',
              )
            }
          />
        ))}
      </div>
      <p sx={stylexStyles.rule37}>{labels[strength]}</p>
    </div>
  )
}
