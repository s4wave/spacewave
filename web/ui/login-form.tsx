import React, {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'
import { LuFingerprint, LuGithub, LuKeyRound } from 'react-icons/lu'
import { FcGoogle } from 'react-icons/fc'
import { RxArrowRight } from 'react-icons/rx'
import { PiUserCircleDuotone } from 'react-icons/pi'
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

// dnsLabelRegex validates DNS label format for usernames.
const dnsLabelRegex = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

const labelClassName = 'text-foreground-alt mb-1.5 block text-xs select-none'

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
      className={cn(
        'group rounded-md border transition-all duration-300',
        fullWidth ? 'w-full' : 'flex-1',
        'border-foreground/10 bg-background/20',
        enabled
          ? 'hover:border-brand/30 hover:bg-background/40'
          : 'cursor-not-allowed opacity-40',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'flex h-11 items-center justify-center gap-2',
      )}
    >
      {loading ? <Spinner size="md" className="text-foreground-alt" /> : icon}
      <span className="text-foreground-alt text-sm">{label}</span>
    </button>
  )
}

// Divider renders a centered label over a horizontal rule.
function Divider({ label }: { label: string }) {
  return (
    <div className="relative py-2">
      <div className="absolute inset-0 flex items-center">
        <div className="border-foreground/10 w-full border-t" />
      </div>
      <div className="relative flex justify-center">
        <span className="bg-background-get-started text-foreground-alt px-3 text-xs">
          {label}
        </span>
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
    <div className="text-foreground-alt text-center text-xs leading-relaxed">
      {creatingAccount ? (
        'creating a new cloud account'
      ) : (
        <>
          local-first, end-to-end encrypted,{' '}
          <span className="text-white">no account required</span>
        </>
      )}
    </div>
  )
}

// useRateLimitCountdown counts down a server rate limit and remembers that the
// password submit should be retried once the countdown ends.
function useRateLimitCountdown() {
  const [countdown, setCountdown] = useState(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingRetryRef = useRef(false)

  useEffect(() => {
    return () => clearTimeout(timerRef.current ?? undefined)
  }, [])

  const start = useCallback((seconds: number) => {
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
  const [prompt, setPrompt] = useState<BrowserSignInAction | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const clearPrompt = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    setPrompt(null)
  }, [])

  const cancel = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    clearPrompt()
    setLoading(null)
  }, [clearPrompt, setLoading])

  useEffect(() => {
    return () => {
      abortRef.current?.abort()
      clearTimeout(timerRef.current ?? undefined)
    }
  }, [])

  const { onBrowserAuth, onContinueWithPasskey, onSignInWithSSO } = handlers
  const start = useCallback(
    async (action: BrowserSignInAction) => {
      const runner = browserSignInRunner(action, {
        onBrowserAuth,
        onContinueWithPasskey,
        onSignInWithSSO,
      })
      if (!runner) return

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
  const [pemFileName, setPemFileName] = useState<string | null>(null)

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
    <div className="space-y-3">
      <div>
        <label className={labelClassName} htmlFor={usernameId}>
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
          className={inputClassName}
        />
      </div>

      <div>
        <label className={labelClassName} htmlFor={passwordId}>
          Password
        </label>
        <input
          id={passwordId}
          type="password"
          value={password}
          onChange={(e) => onPasswordChange(e.target.value)}
          placeholder="Enter password"
          disabled={busy}
          className={inputClassName}
        />
      </div>

      {creatingAccount && (
        <div>
          <label className={labelClassName} htmlFor={confirmId}>
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
            className={cn(
              inputClassName,
              confirm.length > 0 && !passwordsMatch && 'border-destructive/50',
            )}
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
    <label className="flex cursor-pointer items-start gap-2 select-none">
      <input
        type="checkbox"
        checked={agreed}
        onChange={(e) => onChange(e.target.checked)}
        disabled={busy}
        className="accent-brand mt-0.5 size-4 shrink-0 rounded"
      />
      <span className="text-foreground-alt text-xs leading-relaxed">
        I agree to the{' '}
        <a
          href="#/tos"
          className="text-brand inline-flex min-h-11 items-center hover:underline"
          onClick={(e) => e.stopPropagation()}
        >
          Terms of Service
        </a>{' '}
        and{' '}
        <a
          href="#/privacy"
          className="text-brand inline-flex min-h-11 items-center hover:underline"
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
          <p className="text-destructive text-xs">{error}</p>
          {error.includes('Wrong password') && (
            <a
              href={forgotPasswordUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground inline-flex min-h-11 items-center text-xs underline"
            >
              Forgot your password?
            </a>
          )}
        </div>
      )}

      {rateLimitCountdown > 0 && (
        <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3">
          <p className="text-foreground text-xs">
            Rate limited, retrying in {rateLimitCountdown}s…
          </p>
        </div>
      )}

      {browserAuthRequired && (
        <div className="border-brand/30 bg-brand/10 rounded-md border p-3">
          <p className="text-foreground mb-2 text-xs">
            Enhanced security required. Sign in via your browser.
          </p>
          <button
            type="button"
            onClick={onBrowserAuth}
            className={cn(
              'w-full rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/20 hover:bg-brand/30',
              'flex h-11 items-center justify-center gap-2',
            )}
          >
            <LuKeyRound className="text-foreground size-4" />
            <span className="text-foreground text-sm">
              Open browser to sign in…
            </span>
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
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'disabled:cursor-not-allowed disabled:border-foreground/10 disabled:bg-foreground/5 disabled:opacity-60 disabled:hover:bg-foreground/5',
          'flex h-11 items-center justify-center gap-2',
        )}
      >
        {busy ? (
          <Spinner className="text-foreground" />
        ) : (
          <LuKeyRound className="text-foreground size-4" />
        )}
        <span className="text-foreground text-sm">{label}</span>
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
  google: { label: 'Google', icon: <FcGoogle className="size-5" /> },
  github: {
    label: 'GitHub',
    icon: <LuGithub className="text-foreground-alt size-5" />,
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
  const pemInputRef = useRef<HTMLInputElement>(null)
  const busy = loading !== null
  let pemLabel = 'Backup key (.pem)'
  if (pemFileName) pemLabel = `Backup key: ${pemFileName}`
  if (loading === 'pem') pemLabel = 'Signing in with backup key...'

  return (
    <div className="space-y-2">
      <input
        ref={pemInputRef}
        type="file"
        accept=".pem"
        onChange={onPemFileChange}
        className="hidden"
      />
      <SignInMethodButton
        fullWidth
        enabled={canUsePem}
        busy={busy}
        loading={loading === 'pem'}
        icon={<LuKeyRound className="text-foreground-alt size-5" />}
        label={pemLabel}
        onClick={() => pemInputRef.current?.click()}
      />
      <div className="flex gap-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <SignInMethodButton
              enabled={canUsePasskey}
              busy={busy}
              loading={loading === 'passkey'}
              icon={<LuFingerprint className="text-foreground-alt size-5" />}
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
            className={cn(
              'group relative w-full overflow-hidden rounded-md border transition-all duration-300',
              'border-foreground/20 bg-background/20 hover:border-brand/30 hover:bg-background/40',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-11 items-center justify-between px-4',
            )}
          >
            <div className="flex items-center gap-3">
              <PiUserCircleDuotone className="text-foreground-alt group-hover:text-brand size-5 transition-colors" />
              <span className="text-foreground-alt group-hover:text-foreground text-sm transition-colors">
                {loading === 'continue'
                  ? 'Starting…'
                  : 'Continue without account'}
              </span>
            </div>
            <RxArrowRight
              className={cn(
                'text-foreground-alt group-hover:text-brand size-4 transition-all duration-300',
                'group-hover:translate-x-1',
              )}
            />
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="max-w-xs">
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
            className={cn(
              'min-h-11 rounded-md border px-4 py-2 text-sm transition-colors',
              'border-brand/30 bg-brand/10 text-foreground hover:bg-brand/20',
            )}
          >
            Open again
          </button>
          <button
            type="button"
            onClick={onCancel}
            className={cn(
              'min-h-11 rounded-md border px-4 py-2 text-sm transition-colors',
              'border-foreground/20 bg-background text-foreground-alt hover:text-foreground',
            )}
          >
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
  const [loading, setLoadingState] = useState<string | null>(null)
  const [username, setUsername] = useState(
    () => initialUsername?.toLowerCase() ?? '',
  )
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<LoginMode>('login')
  const [agreed, setAgreed] = useState(false)
  const [browserAuthRequired, setBrowserAuthRequired] = useState(false)
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

  const usernameValid = dnsLabelRegex.test(username)
  const passwordValid = password.length >= 8
  const passwordsMatch = password === confirm

  const onAuthBusyChangeRef = useLatestRef(onAuthBusyChange)
  const setLoading = useCallback(
    (value: string | null) => {
      setLoadingState(value)
      onAuthBusyChangeRef.current?.(value !== null)
    },
    [onAuthBusyChangeRef],
  )

  const notifyAuthIdleOnUnmount = useEffectEvent(() => {
    onAuthBusyChange?.(false)
  })

  useEffect(() => {
    return () => notifyAuthIdleOnUnmount()
  }, [])

  const rateLimit = useRateLimitCountdown()

  const getTurnstileToken = useCallback(async (): Promise<string> => {
    if (isDesktop) return ''
    const token = (await turnstileRef.current?.getResponsePromise()) ?? ''
    if (!token) throw new Error('Turnstile verification failed')
    return token
  }, [])

  const startRateLimit = rateLimit.start
  const handleAuthError = useCallback(
    (err: unknown) => {
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
    const invalid = passwordFormError(passwordForm)
    if (invalid) {
      setError(invalid)
      return
    }

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
      className={cn('flex flex-col gap-4', className)}
      onKeyDown={handleKeyDown}
      {...props}
    >
      <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
        <div className="space-y-3 p-6">
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
              className="text-foreground-alt hover:text-brand min-h-11 w-full text-center text-xs transition-colors"
            >
              &larr; Return to login
            </button>
          )}

          {!passwordBusy && mode === 'login' && (
            <>
              <Divider label="or sign in with" />
              <SignInMethods
                loading={loading}
                pemFileName={actions.pemFileName}
                canUsePem={!!onLoginWithPem}
                canUsePasskey={!!onContinueWithPasskey}
                ssoEnabled={ssoEnabled}
                onPemFileChange={actions.changePemFile}
                onPasskey={actions.signInWithPasskey}
                onSSO={actions.signInWithSSO}
              />
              {onContinueWithoutAccount && (
                <ContinueWithoutAccount
                  loading={loading}
                  onContinue={actions.continueWithoutAccount}
                />
              )}
            </>
          )}
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

  if (password.length === 0) return null

  return (
    <div className="space-y-1">
      <div className="bg-foreground/10 flex h-1 gap-0.5 overflow-hidden rounded-full">
        {[1, 2, 3].map((level) => (
          <div
            key={level}
            className={cn(
              'h-full flex-1 rounded-full transition-colors',
              level <= strength ? colors[strength] : 'bg-transparent',
            )}
          />
        ))}
      </div>
      <p className="text-foreground-alt/60 text-xs">{labels[strength]}</p>
    </div>
  )
}
