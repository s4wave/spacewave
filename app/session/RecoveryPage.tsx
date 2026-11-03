import { isDesktop } from '@aptre/bldr'
import { useCallback, useId, useMemo, useRef, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCircleCheck,
  LuKeyRound,
  LuRefreshCw,
  LuShieldCheck,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { Turnstile, type TurnstileInstance } from '@s4wave/web/ui/turnstile.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'

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

const stylexStyles5 = stylex.create({
  rule1: {
    height: '100%',
    flex: 1,
    borderRadius: '3.40282e38px',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles4 = stylex.create({
  rule1: { backgroundColor: themeColors['--color-destructive'] },
  rule2: { backgroundColor: themeColors['--color-yellow-500'] },
  rule3: { backgroundColor: themeColors['--color-green-500'] },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
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
  rule2: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule3: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule4: {
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
  rule5: { padding: `calc(${themeLayout['--spacing']} * 6)` },
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule9: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule10: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule12: {
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
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule14: {
    width: '100%',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
  rule17: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule19: {
    width: '100%',
    cursor: 'not-allowed',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    opacity: 0.7,
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule20: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
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
  },
  rule21: {
    display: 'flex',
    height: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule22: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule23: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule24: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule25: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
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
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
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
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
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
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-destructive'],
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-brand'],
  },
})

type RecoveryStep = 'verifying' | 'form' | 'executing' | 'done'

// parseRecoveryToken extracts the token query parameter from either the hash
// route or the pathname query string for backward compatibility.
// Strips the token from the URL immediately to prevent log/referrer exposure.
function parseRecoveryToken(): string {
  const hash = window.location.hash
  const qIdx = hash.indexOf('?')
  if (qIdx !== -1) {
    const params = new URLSearchParams(hash.slice(qIdx))
    const token = params.get('token')
    if (token) {
      // Strip token from hash query string.
      params.delete('token')
      const remaining = params.toString()
      const newHash = hash.slice(0, qIdx) + (remaining ? '?' + remaining : '')
      window.history.replaceState(
        {},
        '',
        window.location.pathname + window.location.search + newHash,
      )
      return token
    }
  }
  const searchParams = new URLSearchParams(window.location.search)
  const token = searchParams.get('token') ?? ''
  if (token) {
    // Strip token from pathname query string.
    searchParams.delete('token')
    const remaining = searchParams.toString()
    const newUrl =
      window.location.pathname +
      (remaining ? '?' + remaining : '') +
      window.location.hash
    window.history.replaceState({}, '', newUrl)
  }
  return token
}

// RecoveryPage renders the account recovery flow for password reset.
// Users arrive via an email link: spacewave.app/#/recover?token=xyz
export function RecoveryPage() {
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const token = useMemo(function parseToken() {
    return parseRecoveryToken()
  }, [])
  const turnstileRef = useRef<TurnstileInstance>(null)

  const [step, setStep] = useState<RecoveryStep>('verifying')
  const [email, setEmail] = useState('')
  const [requesting, setRequesting] = useState(false)
  const [requestSent, setRequestSent] = useState(false)
  const [requestError, setRequestError] = useState<string | null>(null)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [executeError, setExecuteError] = useState<string | null>(null)
  const turnstileSiteKey = cloudProviderConfig?.turnstileSiteKey ?? ''
  const turnstileReady = isDesktop || turnstileSiteKey !== ''

  // Verify the token on mount.
  const verifyCallback = useCallback(
    async (signal: AbortSignal) => {
      if (!root || !token) return undefined
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      return sw.recoverVerify(token, signal)
    },
    [root, token],
  )
  const verifyState = usePromise(verifyCallback)

  // Transition from verifying to form when verification succeeds.
  const effectiveStep = useMemo((): RecoveryStep => {
    if (step === 'verifying') {
      if (verifyState.data) return 'form'
      return 'verifying'
    }
    return step
  }, [step, verifyState.data])

  const username = verifyState.data?.entityId ?? ''
  const passwordValid = password.length >= 8
  const passwordsMatch = password === confirm

  const handleEmailChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setEmail(e.target.value)
      setRequestError(null)
    },
    [],
  )

  const handleExecute = useCallback(async () => {
    if (!root || !token || !passwordValid || !passwordsMatch) return
    setStep('executing')
    setExecuteError(null)
    try {
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      await sw.recoverExecute({
        token,
        username,
        newPassword: password,
      })
      setStep('done')
    } catch (err) {
      setExecuteError(err instanceof Error ? err.message : 'An error occurred')
      setStep('form')
    }
  }, [root, token, username, password, passwordValid, passwordsMatch])

  const handleRequestRecovery = useCallback(async () => {
    const trimmedEmail = email.trim().toLowerCase()
    if (!root) return
    if (!trimmedEmail.includes('@') || trimmedEmail.length < 5) {
      setRequestError('Enter a valid email address.')
      return
    }
    if (!turnstileReady) {
      setRequestError('Loading server configuration')
      return
    }
    setRequesting(true)
    setRequestError(null)
    try {
      const turnstileToken = isDesktop
        ? ''
        : ((await turnstileRef.current?.takeToken()) ?? '')
      if (!isDesktop && !turnstileToken) {
        throw new Error('Turnstile verification failed')
      }
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      await sw.requestRecoveryEmail({ email: trimmedEmail, turnstileToken })
      setRequestSent(true)
    } catch (err) {
      setRequestError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setRequesting(false)
    }
  }, [email, root, turnstileReady])

  const handleGoToLogin = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const handlePasswordChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setPassword(e.target.value)
      setExecuteError(null)
    },
    [],
  )

  const handleConfirmChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setConfirm(e.target.value)
      setExecuteError(null)
    },
    [],
  )

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' && !token && !requestSent) {
        void handleRequestRecovery()
      }
      if (event.key === 'Enter' && effectiveStep === 'form') {
        void handleExecute()
      }
      if (event.key === 'Escape') {
        handleBack()
      }
    },
    [
      effectiveStep,
      handleExecute,
      handleBack,
      handleRequestRecovery,
      requestSent,
      token,
    ],
  )

  const refCallback = useCallback((node: HTMLDivElement | null) => {
    if (node) {
      node.focus()
    }
  }, [])

  return (
    <AuthScreenLayout
      ref={refCallback}
      onKeyDown={handleKeyDown}
      tabIndex={-1}
      renderTopLeft={(buttonSx) => (
        <button
          type="button"
          onClick={handleBack}
          sx={[stylexStyles3.rule1, buttonSx]}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          <span sx={stylexStyles3.rule2}>Back to home</span>
        </button>
      )}
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles3.rule3}>
            {token ? 'Reset Password' : 'Recover Account'}
          </h1>
        </>
      }
    >
      <div sx={stylexStyles3.rule4}>
        <div sx={stylexStyles3.rule5}>
          {!token && (
            <RequestRecoveryForm
              email={email}
              error={requestError}
              loading={requesting}
              requestSent={requestSent}
              turnstileRef={turnstileRef}
              turnstileSiteKey={turnstileSiteKey}
              onEmailChange={handleEmailChange}
              onGoToLogin={handleGoToLogin}
              onSubmit={handleRequestRecovery}
            />
          )}

          {token && effectiveStep === 'verifying' && (
            <VerifyingStep
              loading={verifyState.loading}
              error={verifyState.error}
              onGoToLogin={handleGoToLogin}
            />
          )}

          {token && effectiveStep === 'form' && (
            <PasswordForm
              username={username}
              password={password}
              confirm={confirm}
              passwordValid={passwordValid}
              passwordsMatch={passwordsMatch}
              error={executeError}
              onPasswordChange={handlePasswordChange}
              onConfirmChange={handleConfirmChange}
              onSubmit={handleExecute}
            />
          )}

          {token && effectiveStep === 'executing' && <ExecutingStep />}

          {token && effectiveStep === 'done' && (
            <DoneStep onGoToLogin={handleGoToLogin} />
          )}
        </div>
      </div>
    </AuthScreenLayout>
  )
}

interface RequestRecoveryFormProps {
  email: string
  error: string | null
  loading: boolean
  requestSent: boolean
  turnstileRef: React.RefObject<TurnstileInstance | null>
  turnstileSiteKey: string
  onEmailChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  onGoToLogin: () => void
  onSubmit: () => void | Promise<void>
}

function RequestRecoveryForm({
  email,
  error,
  loading,
  requestSent,
  turnstileRef,
  turnstileSiteKey,
  onEmailChange,
  onGoToLogin,
  onSubmit,
}: RequestRecoveryFormProps) {
  const emailInputId = useId()
  const handleEmailInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  if (requestSent) {
    return (
      <div>
        <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule6]}>
          <div sx={stylexStyles3.rule7}>
            <LuCircleCheck sx={stylexStyles.rule2} />
          </div>
          <h2 sx={stylexStyles3.rule8}>Check your email</h2>
          <p sx={stylexStyles3.rule9}>
            If the address is attached to a verified account, we sent a recovery
            link. It may take a minute to arrive.
          </p>
        </div>
        <button
          type="button"
          onClick={onGoToLogin}
          sx={[
            nativeChildSpacingNextNextNext.s4,
            [[stylexStyles2.rule1, groupMarker]],
          ]}
        >
          <span sx={stylexStyles3.rule10}>Back to login</span>
          <LuArrowRight sx={stylexStyles.rule3} />
        </button>
      </div>
    )
  }

  return (
    <div>
      <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule6]}>
        <div sx={stylexStyles3.rule7}>
          <LuKeyRound sx={stylexStyles.rule2} />
        </div>
        <h2 sx={stylexStyles3.rule8}>Send a recovery link</h2>
        <p sx={stylexStyles3.rule9}>
          Enter the verified email address on your account and we will send you
          a password reset link.
        </p>
      </div>
      <div sx={nativeChildSpacingNextNextNext.s4}>
        <div sx={nativeChildSpacingNextNext.s3}>
          <label htmlFor={emailInputId} sx={stylexStyles3.rule11}>
            Email
          </label>
          <input
            ref={handleEmailInputRef}
            id={emailInputId}
            type="email"
            value={email}
            onChange={onEmailChange}
            placeholder="you@example.com"
            sx={[stylexStyles3.rule12]}
          />
        </div>
        {!isDesktop && turnstileSiteKey !== '' && (
          <Turnstile
            sx={nativeChildSpacingNextNext.s3}
            ref={turnstileRef}
            siteKey={turnstileSiteKey}
          />
        )}
      </div>
      {error && (
        <p sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule13]}>
          {error}
        </p>
      )}
      <button
        type="button"
        onClick={() => void onSubmit()}
        sx={[
          nativeChildSpacingNextNextNext.s4,
          [[stylexStyles2.rule2, groupMarker]],
        ]}
        disabled={loading}
      >
        {loading ? (
          <Spinner variant="foreground" />
        ) : (
          <LuArrowRight sx={stylexStyles.rule4} />
        )}
        <span sx={stylexStyles3.rule10}>
          {loading ? 'Sending recovery link…' : 'Send recovery link'}
        </span>
      </button>
      <button
        type="button"
        onClick={onGoToLogin}
        sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule14]}
      >
        Back to login
      </button>
    </div>
  )
}

interface VerifyingStepProps {
  loading: boolean
  error: Error | null
  onGoToLogin: () => void
}

function VerifyingStep({ loading, error, onGoToLogin }: VerifyingStepProps) {
  if (loading) {
    return (
      <div>
        <div sx={[nativeChildSpacing.s4, stylexStyles3.rule6]}>
          <Spinner size="lg" variant="muted" />
          <p sx={stylexStyles3.rule15}>Verifying recovery token…</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule6]}>
          <div sx={stylexStyles3.rule16}>
            <LuKeyRound sx={stylexStyles.rule5} />
          </div>
          <h2 sx={stylexStyles3.rule8}>Token verification failed</h2>
          <p sx={stylexStyles3.rule17}>{error.message}</p>
          <p sx={stylexStyles3.rule9}>
            The recovery link may have expired. Please request a new one.
          </p>
        </div>
        <button
          type="button"
          onClick={onGoToLogin}
          sx={[
            nativeChildSpacingNextNextNext.s4,
            [[stylexStyles2.rule1, groupMarker]],
          ]}
        >
          <span sx={stylexStyles3.rule10}>Back to login</span>
          <LuArrowRight sx={stylexStyles.rule3} />
        </button>
      </div>
    )
  }

  return null
}

interface PasswordFormProps {
  username: string
  password: string
  confirm: string
  passwordValid: boolean
  passwordsMatch: boolean
  error: string | null
  onPasswordChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  onConfirmChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  onSubmit: () => void | Promise<void>
}

function PasswordForm({
  username,
  password,
  confirm,
  passwordValid,
  passwordsMatch,
  error,
  onPasswordChange,
  onConfirmChange,
  onSubmit,
}: PasswordFormProps) {
  const canSubmit = passwordValid && passwordsMatch
  const usernameInputId = useId()
  const passwordInputId = useId()
  const confirmInputId = useId()
  const handlePasswordInputRef = useCallback(
    (node: HTMLInputElement | null) => {
      node?.focus()
    },
    [],
  )

  return (
    <div>
      <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule6]}>
        <div sx={stylexStyles3.rule7}>
          <LuShieldCheck sx={stylexStyles.rule2} />
        </div>
        <h2 sx={stylexStyles3.rule8}>Set a new password</h2>
        <p sx={stylexStyles3.rule18}>Choose a new password for your account.</p>
      </div>

      <div sx={nativeChildSpacingNextNextNext.s4}>
        <div sx={nativeChildSpacingNext.s3}>
          <label htmlFor={usernameInputId} sx={stylexStyles3.rule11}>
            Username
          </label>
          <input
            id={usernameInputId}
            type="text"
            value={username}
            readOnly
            sx={[stylexStyles3.rule19]}
          />
        </div>

        <div sx={nativeChildSpacingNext.s3}>
          <label htmlFor={passwordInputId} sx={stylexStyles3.rule11}>
            New password
          </label>
          <input
            ref={handlePasswordInputRef}
            id={passwordInputId}
            type="password"
            value={password}
            onChange={onPasswordChange}
            placeholder="Enter new password"
            sx={[stylexStyles3.rule12]}
          />
        </div>

        <div sx={nativeChildSpacingNext.s3}>
          <label htmlFor={confirmInputId} sx={stylexStyles3.rule11}>
            Confirm new password
          </label>
          <input
            id={confirmInputId}
            type="password"
            value={confirm}
            onChange={onConfirmChange}
            placeholder="Confirm new password"
            sx={
              confirm.length > 0 && !passwordsMatch
                ? stylexStyles3.rule20
                : stylexStyles3.rule12
            }
          />
        </div>

        {password.length > 0 && (
          <PasswordStrength
            sx={nativeChildSpacingNext.s3}
            password={password}
          />
        )}
      </div>

      {error && (
        <p sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule13]}>
          {error}
        </p>
      )}

      <button
        type="button"
        onClick={() => void onSubmit()}
        disabled={!canSubmit}
        sx={[
          nativeChildSpacingNextNextNext.s4,
          [[stylexStyles2.rule2, groupMarker]],
        ]}
      >
        <LuRefreshCw sx={stylexStyles.rule4} />
        <span sx={stylexStyles3.rule10}>Reset password</span>
      </button>
    </div>
  )
}

function PasswordStrength({
  sx,
  password,
}: { password: string } & { sx?: stylex.StyleXStyles; className?: never }) {
  const strength =
    password.length === 0
      ? 0
      : password.length < 8
        ? 1
        : password.length < 12
          ? 2
          : 3
  const labels = ['', 'Weak', 'Fair', 'Strong']
  const colors: stylex.StyleXStyles[] = [
    false,
    stylexStyles4.rule1,
    stylexStyles4.rule2,
    stylexStyles4.rule3,
  ]

  if (password.length === 0) return null

  return (
    <div sx={sx}>
      <div sx={[nativeChildSpacing.s1, stylexStyles3.rule21]}>
        {[1, 2, 3].map((level) => (
          <div
            key={level}
            sx={[
              [stylexStyles5.rule1],
              [
                level <= strength
                  ? colors[strength]
                  : passwordStrengthStyles.empty,
              ],
            ]}
          />
        ))}
      </div>
      <p sx={[nativeChildSpacing.s1, stylexStyles3.rule22]}>
        {labels[strength]}
      </p>
    </div>
  )
}

function ExecutingStep() {
  return (
    <div>
      <div sx={[nativeChildSpacing.s4, stylexStyles3.rule23]}>
        <Spinner size="lg" variant="muted" />
        <h2 sx={stylexStyles3.rule8}>Resetting your password…</h2>
        <p sx={stylexStyles3.rule18}>
          Deriving new encryption keys. This may take a moment.
        </p>
      </div>
    </div>
  )
}

function DoneStep({ onGoToLogin }: { onGoToLogin: () => void }) {
  return (
    <div>
      <div sx={[nativeChildSpacingNextNextNext.s4, stylexStyles3.rule23]}>
        <div sx={stylexStyles3.rule24}>
          <LuCircleCheck sx={stylexStyles.rule6} />
        </div>
        <h2 sx={stylexStyles3.rule8}>Password reset successfully!</h2>
        <p sx={stylexStyles3.rule25}>
          You can now sign in with your new password.
        </p>
      </div>

      <button
        type="button"
        onClick={onGoToLogin}
        sx={[
          nativeChildSpacingNextNextNext.s4,
          [[stylexStyles2.rule1, groupMarker]],
        ]}
      >
        <span sx={stylexStyles3.rule10}>Go to login</span>
        <LuArrowRight sx={stylexStyles.rule3} />
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// passwordStrengthStyles leaves unused strength segments transparent.
const passwordStrengthStyles = stylex.create({
  empty: { backgroundColor: 'transparent' },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNextNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
