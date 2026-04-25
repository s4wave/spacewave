import { isDesktop } from '@aptre/bldr'
import { useCallback, useMemo, useRef, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCircleCheck,
  LuKeyRound,
  LuRefreshCw,
  LuShieldCheck,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { Turnstile, type TurnstileInstance } from '@s4wave/web/ui/turnstile.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'

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
  const token = useMemo(parseRecoveryToken, [])
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
      const turnstileToken =
        isDesktop ? '' : (
          ((await turnstileRef.current?.getResponsePromise()) ?? '')
        )
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
      topLeft={
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-brand flex items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          <span className="select-none">Back to home</span>
        </button>
      }
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h1 className="text-xl font-bold tracking-wide">
            {token ? 'Reset Password' : 'Recover Account'}
          </h1>
        </>
      }
    >
      <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
        <div className="p-6">
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
  if (requestSent) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
            <LuCircleCheck className="text-brand h-5 w-5" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Check your email
          </h2>
          <p className="text-foreground-alt text-center text-xs leading-relaxed">
            If the address is attached to a verified account, we sent a recovery
            link. It may take a minute to arrive.
          </p>
        </div>
        <button
          onClick={onGoToLogin}
          className={cn(
            'group w-full rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <span className="text-foreground text-sm">Back to login</span>
          <LuArrowRight className="text-foreground-alt h-4 w-4" />
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-2">
        <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
          <LuKeyRound className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Send a recovery link
        </h2>
        <p className="text-foreground-alt text-center text-xs leading-relaxed">
          Enter the verified email address on your account and we will send you
          a password reset link.
        </p>
      </div>
      <div className="space-y-3">
        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Email
          </label>
          <input
            type="email"
            value={email}
            onChange={onEmailChange}
            placeholder="you@example.com"
            autoFocus
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
              'focus:border-brand/50',
            )}
          />
        </div>
        {!isDesktop && turnstileSiteKey !== '' && (
          <Turnstile ref={turnstileRef} siteKey={turnstileSiteKey} />
        )}
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
      <button
        onClick={() => void onSubmit()}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'flex h-10 items-center justify-center gap-2',
        )}
        disabled={loading}
      >
        {loading ?
          <Spinner className="text-foreground" />
        : <LuArrowRight className="text-foreground h-4 w-4" />}
        <span className="text-foreground text-sm">
          {loading ? 'Sending recovery link...' : 'Send recovery link'}
        </span>
      </button>
      <button
        onClick={onGoToLogin}
        className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
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
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <Spinner size="lg" className="text-foreground-alt" />
          <p className="text-foreground-alt text-sm">
            Verifying recovery token...
          </p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-destructive/10 flex h-10 w-10 items-center justify-center rounded-full">
            <LuKeyRound className="text-destructive h-5 w-5" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Token verification failed
          </h2>
          <p className="text-destructive text-center text-xs">
            {error.message}
          </p>
          <p className="text-foreground-alt text-center text-xs leading-relaxed">
            The recovery link may have expired. Please request a new one.
          </p>
        </div>
        <button
          onClick={onGoToLogin}
          className={cn(
            'group w-full rounded-md border transition-all duration-300',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <span className="text-foreground text-sm">Back to login</span>
          <LuArrowRight className="text-foreground-alt h-4 w-4" />
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

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-2">
        <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
          <LuShieldCheck className="text-brand h-5 w-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Set a new password
        </h2>
        <p className="text-foreground-alt text-xs">
          Choose a new password for your account.
        </p>
      </div>

      <div className="space-y-3">
        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Username
          </label>
          <input
            type="text"
            value={username}
            readOnly
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground w-full rounded-md border px-3 py-2 text-sm outline-none',
              'cursor-not-allowed opacity-70',
            )}
          />
        </div>

        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            New password
          </label>
          <input
            type="password"
            value={password}
            onChange={onPasswordChange}
            placeholder="Enter new password"
            autoFocus
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
              'focus:border-brand/50',
            )}
          />
        </div>

        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Confirm new password
          </label>
          <input
            type="password"
            value={confirm}
            onChange={onConfirmChange}
            placeholder="Confirm new password"
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
              'focus:border-brand/50',
              confirm.length > 0 && !passwordsMatch && 'border-destructive/50',
            )}
          />
        </div>

        {password.length > 0 && <PasswordStrength password={password} />}
      </div>

      {error && <p className="text-destructive text-xs">{error}</p>}

      <button
        onClick={() => void onSubmit()}
        disabled={!canSubmit}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <LuRefreshCw className="text-foreground h-4 w-4" />
        <span className="text-foreground text-sm">Reset password</span>
      </button>
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

function ExecutingStep() {
  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-3">
        <Spinner size="lg" className="text-foreground-alt" />
        <h2 className="text-foreground text-sm font-medium">
          Resetting your password...
        </h2>
        <p className="text-foreground-alt text-xs">
          Deriving new encryption keys. This may take a moment.
        </p>
      </div>
    </div>
  )
}

function DoneStep({ onGoToLogin }: { onGoToLogin: () => void }) {
  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-3">
        <div className="bg-brand/10 flex h-12 w-12 items-center justify-center rounded-full">
          <LuCircleCheck className="text-brand h-6 w-6" />
        </div>
        <h2 className="text-foreground text-sm font-medium">
          Password reset successfully!
        </h2>
        <p className="text-foreground-alt text-xs leading-relaxed">
          You can now sign in with your new password.
        </p>
      </div>

      <button
        onClick={onGoToLogin}
        className={cn(
          'group w-full rounded-md border transition-all duration-300',
          'border-brand/30 bg-brand/10 hover:bg-brand/20',
          'flex h-10 items-center justify-center gap-2',
        )}
      >
        <span className="text-foreground text-sm">Go to login</span>
        <LuArrowRight className="text-foreground-alt h-4 w-4" />
      </button>
    </div>
  )
}
