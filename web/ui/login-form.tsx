import React, { useCallback, useEffect, useRef, useState } from 'react'
import { LuFingerprint, LuGithub, LuKeyRound } from 'react-icons/lu'
import { FcGoogle } from 'react-icons/fc'
import { RxArrowRight } from 'react-icons/rx'
import { PiUserCircleDuotone } from 'react-icons/pi'
import { isDesktop } from '@aptre/bldr'
import type { CloudProviderConfig } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
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
  'w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
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
      disabled={busy || !enabled}
      onClick={onClick}
      className={cn(
        'group rounded-md border transition-all duration-300',
        fullWidth ? 'w-full' : 'flex-1',
        'border-foreground/10 bg-background/20',
        enabled ?
          'hover:border-brand/30 hover:bg-background/40'
        : 'cursor-not-allowed opacity-40',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'flex h-10 items-center justify-center gap-2',
      )}
    >
      {loading ?
        <Spinner size="md" className="text-foreground-alt" />
      : icon}
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
  ...props
}: LoginFormProps) {
  const [loading, setLoading] = useState<string | null>(null)
  const [username, setUsername] = useState(initialUsername?.toLowerCase() ?? '')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<LoginMode>('login')
  const [agreed, setAgreed] = useState(false)
  const [rateLimitCountdown, setRateLimitCountdown] = useState(0)
  const [browserAuthRequired, setBrowserAuthRequired] = useState(false)
  const [browserSignInPrompt, setBrowserSignInPrompt] =
    useState<BrowserSignInAction | null>(null)
  const [pemFileName, setPemFileName] = useState<string | null>(null)
  const turnstileRef = useRef<TurnstileInstance>(null)
  const pemInputRef = useRef<HTMLInputElement>(null)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const browserSignInTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  )
  const browserSignInAbortRef = useRef<AbortController | null>(null)
  const turnstileSiteKey = cloudProviderConfig?.turnstileSiteKey ?? ''
  const publicBaseUrl =
    cloudProviderConfig?.publicBaseUrl ?? 'https://spacewave.app'
  const publicHost = stripHost(publicBaseUrl)
  const forgotPasswordUrl = buildHashRouteURL(publicBaseUrl, '/recover')
  const turnstileReady = isDesktop || turnstileSiteKey !== ''
  const creatingAccount = mode === 'confirm_create'
  const googleSsoEnabled =
    !!onSignInWithSSO && !!cloudProviderConfig?.googleSsoEnabled
  const githubSsoEnabled =
    !!onSignInWithSSO && !!cloudProviderConfig?.githubSsoEnabled

  const usernameValid = dnsLabelRegex.test(username)
  const passwordValid = password.length >= 8
  const passwordsMatch = password === confirm

  const wasRateLimitedRef = useRef(false)
  const pendingRetryRef = useRef(false)

  const clearBrowserSignInPrompt = useCallback(() => {
    if (browserSignInTimerRef.current) {
      clearTimeout(browserSignInTimerRef.current)
      browserSignInTimerRef.current = null
    }
    setBrowserSignInPrompt(null)
  }, [])

  const cancelBrowserSignInAttempt = useCallback(() => {
    browserSignInAbortRef.current?.abort()
    browserSignInAbortRef.current = null
    clearBrowserSignInPrompt()
    setLoading(null)
  }, [clearBrowserSignInPrompt])

  useEffect(() => {
    return () => {
      browserSignInAbortRef.current?.abort()
      if (browserSignInTimerRef.current) {
        clearTimeout(browserSignInTimerRef.current)
      }
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current)
      }
    }
  }, [])

  const startRateLimitCountdown = useCallback((seconds: number) => {
    setRateLimitCountdown(seconds)
    setBrowserAuthRequired(false)
    setError(null)
    wasRateLimitedRef.current = true
    pendingRetryRef.current = true
    const tick = () => {
      setRateLimitCountdown((prev) => {
        if (prev <= 1) {
          retryTimerRef.current = null
          return 0
        }
        retryTimerRef.current = setTimeout(tick, 1000)
        return prev - 1
      })
    }
    if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    retryTimerRef.current = setTimeout(tick, 1000)
  }, [])

  const handleAction = useCallback(
    async (action: string, handler?: () => void | Promise<void>) => {
      if (!handler) return
      setLoading(action)
      setError(null)
      try {
        await handler()
      } catch (err) {
        setError(err instanceof Error ? err.message : 'An error occurred')
      } finally {
        setLoading(null)
      }
    },
    [],
  )

  const getTurnstileToken = useCallback(async (): Promise<string> => {
    if (isDesktop) return ''
    const token = (await turnstileRef.current?.getResponsePromise()) ?? ''
    if (!token) throw new Error('Turnstile verification failed')
    return token
  }, [])

  const handleAuthError = useCallback(
    (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'An error occurred'
      if (isBrowserAuthRequired(msg)) {
        setBrowserAuthRequired(true)
        setError(null)
        return
      }
      const retryAfter = parseRateLimitError(msg)
      if (retryAfter > 0) {
        startRateLimitCountdown(retryAfter)
        return
      }
      if (msg.includes('connection refused')) {
        setError(
          'Cannot reach the server. Please check your connection and try again.',
        )
      } else if (!isDesktop && msg.includes('unsupported host')) {
        setError(`Spacewave Cloud sign-in is only supported on ${publicHost}`)
      } else {
        setError(msg)
      }
    },
    [publicHost, startRateLimitCountdown],
  )

  const handleContinueWithPassword = useCallback(async () => {
    if (!usernameValid) {
      setError(
        'Username must be a valid DNS label (lowercase letters, numbers, hyphens)',
      )
      return
    }
    if (!passwordValid) {
      setError('Password must be at least 8 characters')
      return
    }
    if (creatingAccount && !agreed) {
      setError('You must agree to the Terms of Service and Privacy Policy')
      return
    }
    if (!turnstileReady) {
      setError('Loading server configuration')
      return
    }
    if (creatingAccount && !passwordsMatch) {
      setError('Passwords do not match')
      return
    }

    setError(null)
    setLoading('password')
    try {
      const token = await getTurnstileToken()
      if (creatingAccount) {
        const result = await onCreateAccountWithPassword?.(
          username,
          password,
          token,
        )
        if (result) onNavigateToSession?.(result.sessionIndex, true)
      } else {
        const result = await onLoginWithPassword?.(username, password, token)
        if (result) {
          switch (result.type) {
            case 'session':
              onNavigateToSession?.(result.sessionIndex, false)
              break
            case 'new_account':
              setMode('confirm_create')
              break
            case 'error':
              setError(getErrorMessage(result.errorCode, 'password'))
              break
          }
        }
      }
    } catch (err) {
      handleAuthError(err)
    } finally {
      setLoading(null)
    }
  }, [
    usernameValid,
    passwordValid,
    agreed,
    turnstileReady,
    creatingAccount,
    passwordsMatch,
    getTurnstileToken,
    handleAuthError,
    onLoginWithPassword,
    onCreateAccountWithPassword,
    onNavigateToSession,
    username,
    password,
  ])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' && loading === null) {
        void handleContinueWithPassword()
      }
    },
    [handleContinueWithPassword, loading],
  )

  // Auto-retry after rate limit countdown reaches 0.
  if (rateLimitCountdown === 0 && pendingRetryRef.current && loading === null) {
    pendingRetryRef.current = false
    setTimeout(() => void handleContinueWithPassword(), 0)
  }

  const handleUsernameChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setUsername(e.target.value.toLowerCase())
      setError(null)
    },
    [],
  )

  const handlePasswordChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setPassword(e.target.value)
      setError(null)
    },
    [],
  )

  const handleConfirmChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setConfirm(e.target.value)
      setError(null)
    },
    [],
  )

  const handlePemFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result
        if (!(result instanceof ArrayBuffer)) return
        setPemFileName(file.name)
        void handleAction('pem', async () => {
          const login = await onLoginWithPem?.(new Uint8Array(result))
          if (!login) return
          onNavigateToSession?.(login.sessionIndex, false)
        })
      }
      reader.onerror = () => {
        setError('Failed to read backup key')
      }
      reader.readAsArrayBuffer(file)
      e.target.value = ''
    },
    [handleAction, onLoginWithPem, onNavigateToSession],
  )

  const handleDesktopBrowserSignIn = useCallback(
    async (action: BrowserSignInAction) => {
      let runner: ((abortSignal: AbortSignal) => Promise<void>) | undefined
      if (action === 'browser') {
        if (!onBrowserAuth) return
        runner = async (abortSignal) => {
          await onBrowserAuth(abortSignal)
        }
      } else if (action === 'passkey') {
        if (!onContinueWithPasskey) return
        runner = async (abortSignal) => {
          await onContinueWithPasskey(abortSignal)
        }
      } else {
        if (!onSignInWithSSO) return
        runner = async (abortSignal) => {
          await onSignInWithSSO(action, abortSignal)
        }
      }

      browserSignInAbortRef.current?.abort()
      const controller = new AbortController()
      browserSignInAbortRef.current = controller
      clearBrowserSignInPrompt()
      setLoading(action)
      setError(null)
      browserSignInTimerRef.current = setTimeout(() => {
        if (browserSignInAbortRef.current !== controller) return
        if (controller.signal.aborted) return
        setBrowserSignInPrompt(action)
      }, browserSignInPromptDelayMs)

      try {
        await runner(controller.signal)
      } catch (err) {
        if (controller.signal.aborted || isAbortError(err)) return
        handleAuthError(err)
      } finally {
        if (browserSignInAbortRef.current === controller) {
          browserSignInAbortRef.current = null
          clearBrowserSignInPrompt()
          setLoading(null)
        }
      }
    },
    [
      clearBrowserSignInPrompt,
      handleAuthError,
      onBrowserAuth,
      onContinueWithPasskey,
      onSignInWithSSO,
    ],
  )

  return (
    <div
      className={cn('flex flex-col gap-4', className)}
      onKeyDown={handleKeyDown}
      {...props}
    >
      <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
        <div className="space-y-3 p-6">
          <div className="space-y-3">
            <div>
              <label className={labelClassName}>Username</label>
              <input
                type="text"
                value={username}
                onChange={handleUsernameChange}
                placeholder="alice"
                autoFocus
                disabled={loading !== null || creatingAccount}
                className={inputClassName}
              />
            </div>

            <div>
              <label className={labelClassName}>Password</label>
              <input
                type="password"
                value={password}
                onChange={handlePasswordChange}
                placeholder="Enter password"
                disabled={loading !== null}
                className={inputClassName}
              />
            </div>

            {creatingAccount && (
              <div>
                <label className={labelClassName}>Confirm password</label>
                <input
                  type="password"
                  value={confirm}
                  onChange={handleConfirmChange}
                  placeholder="Confirm password"
                  autoFocus
                  disabled={loading !== null}
                  className={cn(
                    inputClassName,
                    confirm.length > 0 &&
                      !passwordsMatch &&
                      'border-destructive/50',
                  )}
                />
              </div>
            )}

            {password.length > 0 && <PasswordStrength password={password} />}
          </div>

          {creatingAccount && (
            <label className="flex cursor-pointer items-start gap-2 select-none">
              <input
                type="checkbox"
                checked={agreed}
                onChange={(e) => setAgreed(e.target.checked)}
                disabled={loading !== null}
                className="accent-brand mt-0.5 h-4 w-4 shrink-0 rounded"
              />
              <span className="text-foreground-alt text-xs leading-relaxed">
                I agree to the{' '}
                <a
                  href="#/tos"
                  className="text-brand hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  Terms of Service
                </a>{' '}
                and{' '}
                <a
                  href="#/privacy"
                  className="text-brand hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  Privacy Policy
                </a>
              </span>
            </label>
          )}

          {error && (
            <div>
              <p className="text-destructive text-xs">{error}</p>
              {error.includes('Wrong password') && (
                <a
                  href={forgotPasswordUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-muted-foreground text-xs underline"
                >
                  Forgot your password?
                </a>
              )}
            </div>
          )}

          {rateLimitCountdown > 0 && (
            <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3">
              <p className="text-foreground text-xs">
                Rate limited, retrying in {rateLimitCountdown}s...
              </p>
            </div>
          )}

          {browserAuthRequired && (
            <div className="border-brand/30 bg-brand/10 rounded-md border p-3">
              <p className="text-foreground mb-2 text-xs">
                Enhanced security required. Sign in via your browser.
              </p>
              <button
                onClick={() =>
                  isDesktop ?
                    void handleDesktopBrowserSignIn('browser')
                  : void onBrowserAuth?.()
                }
                className={cn(
                  'w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/20 hover:bg-brand/30',
                  'flex h-9 items-center justify-center gap-2',
                )}
              >
                <LuKeyRound className="text-foreground h-4 w-4" />
                <span className="text-foreground text-sm">
                  Open browser to sign in...
                </span>
              </button>
            </div>
          )}

          {!isDesktop && turnstileSiteKey !== '' && (
            <Turnstile ref={turnstileRef} siteKey={turnstileSiteKey} />
          )}

          <button
            onClick={() => void handleContinueWithPassword()}
            disabled={
              loading !== null ||
              !usernameValid ||
              !passwordValid ||
              (creatingAccount && !agreed) ||
              !turnstileReady ||
              rateLimitCountdown > 0
            }
            className={cn(
              'group w-full rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            {loading === 'password' ?
              <Spinner className="text-foreground" />
            : <LuKeyRound className="text-foreground h-4 w-4" />}
            <span className="text-foreground text-sm">
              {loading === 'password' ?
                'Connecting...'
              : creatingAccount ?
                'Confirm and create account'
              : 'Continue with password'}
            </span>
          </button>

          {creatingAccount && (
            <button
              type="button"
              onClick={() => {
                setMode('login')
                setConfirm('')
                setError(null)
              }}
              className="text-foreground-alt hover:text-brand w-full text-center text-xs transition-colors"
            >
              &larr; Return to login
            </button>
          )}

          {mode === 'login' && (
            <>
              <Divider label="or sign in with" />

              <div className="space-y-2">
                <input
                  ref={pemInputRef}
                  type="file"
                  accept=".pem"
                  onChange={handlePemFileChange}
                  className="hidden"
                />
                <SignInMethodButton
                  fullWidth
                  enabled={!!onLoginWithPem}
                  busy={loading !== null}
                  loading={loading === 'pem'}
                  icon={<LuKeyRound className="text-foreground-alt h-5 w-5" />}
                  label={
                    loading === 'pem' ? 'Signing in with backup key...'
                    : pemFileName ? `Backup key: ${pemFileName}`
                    : 'Backup key (.pem)'
                  }
                  onClick={() => pemInputRef.current?.click()}
                />
                <div className="flex gap-2">
                  {[
                    {
                      action: 'passkey',
                      enabled: !!onContinueWithPasskey,
                      icon: (
                        <LuFingerprint className="text-foreground-alt h-5 w-5" />
                      ),
                      label: 'Passkey',
                      onClick: () =>
                        isDesktop ?
                          void handleDesktopBrowserSignIn('passkey')
                        : void handleAction('passkey', onContinueWithPasskey),
                    },
                    googleSsoEnabled ?
                      {
                        action: 'google',
                        enabled: true,
                        icon: <FcGoogle className="h-5 w-5" />,
                        label: 'Google',
                        onClick: () => void onSignInWithSSO?.('google'),
                      }
                    : null,
                    githubSsoEnabled ?
                      {
                        action: 'github',
                        enabled: true,
                        icon: (
                          <LuGithub className="text-foreground-alt h-5 w-5" />
                        ),
                        label: 'GitHub',
                        onClick: () => void onSignInWithSSO?.('github'),
                      }
                    : null,
                  ]
                    .filter((item) => item !== null)
                    .map(({ action, enabled, icon, label, onClick }) => (
                      <Tooltip key={action}>
                        <TooltipTrigger asChild>
                          <SignInMethodButton
                            enabled={enabled}
                            busy={loading !== null}
                            loading={loading === action}
                            icon={icon}
                            label={label}
                            onClick={onClick}
                          />
                        </TooltipTrigger>
                        {!enabled && (
                          <TooltipContent side="bottom">
                            Coming soon
                          </TooltipContent>
                        )}
                      </Tooltip>
                    ))}
                </div>
              </div>

              {onContinueWithoutAccount && (
                <>
                  <Divider label="or" />
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        onClick={() =>
                          void handleAction(
                            'continue',
                            onContinueWithoutAccount,
                          )
                        }
                        disabled={loading !== null}
                        className={cn(
                          'group relative w-full overflow-hidden rounded-md border transition-all duration-300',
                          'border-foreground/20 bg-background/20 hover:border-brand/30 hover:bg-background/40',
                          'disabled:cursor-not-allowed disabled:opacity-50',
                          'flex h-10 items-center justify-between px-4',
                        )}
                      >
                        <div className="flex items-center gap-3">
                          <PiUserCircleDuotone className="text-foreground-alt group-hover:text-brand h-5 w-5 transition-colors" />
                          <span className="text-foreground-alt group-hover:text-foreground text-sm transition-colors">
                            {loading === 'continue' ?
                              'Starting...'
                            : 'Continue without account'}
                          </span>
                        </div>
                        <RxArrowRight
                          className={cn(
                            'text-foreground-alt group-hover:text-brand h-4 w-4 transition-all duration-300',
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
              )}
            </>
          )}
        </div>
      </div>

      <Dialog
        open={browserSignInPrompt !== null}
        onOpenChange={(open) => {
          if (!open) cancelBrowserSignInAttempt()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Continue sign-in in your browser</DialogTitle>
            <DialogDescription>
              Spacewave opened a{' '}
              {getBrowserSignInLabel(browserSignInPrompt ?? 'browser')} sign-in
              page in your web browser. If it did not appear, open it again or
              cancel this attempt.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              type="button"
              onClick={() => {
                if (!browserSignInPrompt) return
                void handleDesktopBrowserSignIn(browserSignInPrompt)
              }}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-colors',
                'border-brand/30 bg-brand/10 text-foreground hover:bg-brand/20',
              )}
            >
              Open again
            </button>
            <button
              type="button"
              onClick={cancelBrowserSignInAttempt}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-colors',
                'border-foreground/20 bg-background text-foreground-alt hover:text-foreground',
              )}
            >
              Cancel attempt
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <div className="text-foreground-alt text-center text-xs leading-relaxed">
        {creatingAccount ?
          'creating a new cloud account'
        : <>
            local-first, end-to-end encrypted,{' '}
            <span className="text-white">no account required</span>
          </>
        }
      </div>
    </div>
  )
}

function PasswordStrength({ password }: { password: string }) {
  const strength =
    password.length === 0 ? 0
    : password.length < 8 ? 1
    : password.length < 12 ? 2
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
