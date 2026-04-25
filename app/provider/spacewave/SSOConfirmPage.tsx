import { useCallback, useMemo, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuArrowLeft, LuCheck, LuUserPlus } from 'react-icons/lu'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { getPendingSSOState, clearPendingSSOState } from './sso-state.js'
import { generateAuthKeypairs, wrapPemWithPin } from './keypair-utils.js'
import { OptionalPinLock } from './OptionalPinLock.js'
import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
  authInputClassName,
  getProviderLabel,
  getErrorMessage,
  isUsernameTakenError,
  loginWithEntityPem,
  normalizeUsernameInput,
  ProviderIcon,
  validateOptionalPin,
  validateUsername,
  withSpacewaveProvider,
} from './auth-flow-shared.js'

type SSOConfirmState =
  | { step: 'form' }
  | { step: 'confirm'; username: string }
  | { step: 'creating' }
  | { step: 'logging_in' }
  | { step: 'error'; message: string }

const SIGNUP_HIGHLIGHTS = ['End-to-end encrypted', 'Local-first', 'Open source']

// SSOConfirmPage handles new-account username entry after SSO.
// Route: /auth/sso/:provider/confirm
// Shared by both desktop and web SSO flows.
export function SSOConfirmPage() {
  const navigate = useNavigate()
  const params = useParams()
  const routeProvider = params?.provider ?? ''
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, setState] = useState<SSOConfirmState>({ step: 'form' })
  const [username, setUsername] = useState('')
  const [usernameError, setUsernameError] = useState('')
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [pinError, setPinError] = useState('')
  const pendingState = useMemo(() => getPendingSSOState(), [])
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')

  const handleUsernameChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = normalizeUsernameInput(e.target.value)
      setUsername(next.username)
      setUsernameError(next.error)
    },
    [],
  )

  const handlePinChange = useCallback((value: string) => {
    setPin(value)
    setPinError('')
  }, [])

  const handleConfirmPinChange = useCallback((value: string) => {
    setConfirmPin(value)
    setPinError('')
  }, [])

  const handleCancel = useCallback(() => {
    clearPendingSSOState()
    navigate({ path: '/login' })
  }, [navigate])

  const handleRestartDesktop = useCallback(() => {
    clearPendingSSOState()
    navigate({ path: `/auth/sso/${routeProvider}` })
  }, [navigate, routeProvider])

  const handleRequestConfirm = useCallback(() => {
    if (!pendingState) {
      setState({ step: 'error', message: 'SSO session expired' })
      return
    }
    const usernameValidationError = validateUsername(username)
    if (usernameValidationError) {
      setUsernameError(usernameValidationError)
      return
    }
    const pinValidationError = validateOptionalPin(pin, confirmPin)
    if (pinValidationError) {
      setPinError(pinValidationError)
      return
    }
    setState({ step: 'confirm', username })
  }, [pendingState, username, pin, confirmPin])

  const handleCancelConfirm = useCallback(() => {
    setState({ step: 'form' })
  }, [])

  const handleCreateAccount = useCallback(async () => {
    if (!pendingState) {
      setState({ step: 'error', message: 'SSO session expired' })
      return
    }
    const usernameValidationError = validateUsername(username)
    if (usernameValidationError) {
      setState({ step: 'form' })
      setUsernameError(usernameValidationError)
      return
    }
    const pinValidationError = validateOptionalPin(pin, confirmPin)
    if (pinValidationError) {
      setState({ step: 'form' })
      setPinError(pinValidationError)
      return
    }
    const wantsPin = pin.length > 0 || confirmPin.length > 0
    if (!root) {
      setState({ step: 'error', message: 'Not connected to server' })
      return
    }

    setState({ step: 'creating' })

    try {
      await withSpacewaveProvider(root, async (spacewave) => {
        const { entity, session } = await generateAuthKeypairs(spacewave)
        const wrappedEntityKey =
          wantsPin ?
            await wrapPemWithPin(spacewave, entity.pem, pin)
          : entity.custodiedPemBase64

        await spacewave.confirmSSO({
          nonce: pendingState.nonce,
          username,
          wrappedEntityKey,
          entityPeerId: entity.peerId,
          sessionPeerId: session.peerId,
          pinWrapped: wantsPin,
        })
        setState({ step: 'logging_in' })
        const sessionIndex = await loginWithEntityPem(
          root,
          new TextEncoder().encode(entity.pem),
        )
        clearPendingSSOState()
        navigate({ path: `/u/${sessionIndex}` })
      })
    } catch (err) {
      if (isUsernameTakenError(err)) {
        setState({ step: 'form' })
        setUsernameError('Username is already taken')
        return
      }
      setState({
        step: 'error',
        message: getErrorMessage(err, 'Account creation failed'),
      })
    }
  }, [pendingState, username, pin, confirmPin, root, navigate])

  // No pending state = expired.
  if (!pendingState) {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Session expired
            </h2>
          </>
        }
      >
        <AuthStatusPanel
          icon={<></>}
          message="Your sign-in session has expired. Please try again."
        >
          <div className="flex w-full flex-col gap-2">
            {isDesktop && routeProvider && (
              <AuthPrimaryActionButton onClick={handleRestartDesktop}>
                Restart sign-in
              </AuthPrimaryActionButton>
            )}
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="h-3 w-3" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthStatusPanel>
      </AuthScreenLayout>
    )
  }

  // Error state.
  if (state.step === 'error') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Account creation failed
            </h2>
          </>
        }
      >
        <div className="flex w-full flex-col items-center gap-4">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Account creation failed',
              error: state.message,
            }}
          />
          <div className="flex w-full flex-col gap-2">
            {isDesktop && routeProvider && (
              <AuthPrimaryActionButton onClick={handleRestartDesktop}>
                Restart sign-in
              </AuthPrimaryActionButton>
            )}
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="h-3 w-3" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </div>
      </AuthScreenLayout>
    )
  }

  // Creating / logging in.
  if (state.step === 'creating' || state.step === 'logging_in') {
    return (
      <AuthScreenLayout
        alwaysShowIntro
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              {state.step === 'logging_in' ?
                'Signing in...'
              : 'Creating account...'}
            </h2>
            {pendingState.email && (
              <p className="text-foreground-alt text-sm">
                {pendingState.email}
              </p>
            )}
          </>
        }
      >
        <LoadingCard
          view={{
            state: 'active',
            title:
              state.step === 'logging_in' ?
                'Signing you in'
              : `Creating ${username}`,
            detail:
              state.step === 'logging_in' ?
                'Mounting your new session.'
              : 'Registering your username with the provider.',
          }}
        />
      </AuthScreenLayout>
    )
  }

  const providerLabel = getProviderLabel(pendingState.provider)
  const confirmOpen = state.step === 'confirm'
  const confirmUsername = state.step === 'confirm' ? state.username : username

  // Form state (with optional confirm modal overlay).
  return (
    <AuthScreenLayout
      alwaysShowIntro
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground text-lg font-semibold">
            Welcome to Spacewave
          </h2>
          <p className="text-foreground-alt text-sm">
            Choose a username to finish signing up
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <AuthCard>
          {/* Provider context header */}
          <div className="mb-4 flex items-center gap-3">
            <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-lg">
              <ProviderIcon
                provider={pendingState.provider}
                className="h-5 w-5"
              />
            </div>
            <div>
              <h2 className="text-foreground text-sm font-semibold">
                Sign up with {providerLabel}
              </h2>
              {pendingState.email && (
                <p className="text-foreground-alt text-xs">
                  {pendingState.email}
                </p>
              )}
            </div>
          </div>

          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              handleRequestConfirm()
            }}
          >
            <label className="flex flex-col gap-1.5">
              <span className="text-foreground-alt text-xs select-none">
                Username
              </span>
              <input
                value={username}
                onChange={handleUsernameChange}
                placeholder="your-name"
                className={cn(
                  authInputClassName,
                  usernameError && 'border-destructive/50',
                )}
                autoFocus
              />
              {usernameError ?
                <p className="text-destructive text-xs">{usernameError}</p>
              : <p className="text-foreground-alt/50 text-xs">
                  Lowercase letters, numbers, and hyphens
                </p>
              }
            </label>
            <OptionalPinLock
              pin={pin}
              confirmPin={confirmPin}
              pinError={pinError}
              onPinChange={handlePinChange}
              onConfirmPinChange={handleConfirmPinChange}
              onSubmit={handleRequestConfirm}
              disabled={false}
              pinInputId="sso-pin"
            />
            <AuthPrimaryActionButton
              type="submit"
              disabled={!username || !!usernameError}
              icon={<LuUserPlus className="text-foreground h-4 w-4" />}
            >
              Create account
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="h-3 w-3" />
              Back to login
            </AuthSecondaryActionButton>
          </form>
        </AuthCard>

        {/* Trust signals */}
        <div className="text-foreground-alt flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-xs">
          {SIGNUP_HIGHLIGHTS.map((text) => (
            <span key={text} className="flex items-center gap-1.5">
              <LuCheck className="text-brand h-3.5 w-3.5" />
              {text}
            </span>
          ))}
        </div>
      </div>

      <Dialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!open) handleCancelConfirm()
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Confirm your username</DialogTitle>
            <DialogDescription>
              Your account will be created as{' '}
              <span className="text-foreground font-semibold">
                {confirmUsername}
              </span>
              . This username is permanent and cannot be changed later.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={() => void handleCreateAccount()}
              icon={<LuUserPlus className="text-foreground h-4 w-4" />}
            >
              Confirm and create account
            </AuthPrimaryActionButton>
            <p className="text-foreground-alt/70 text-center text-xs">
              By clicking Confirm, you agree to our{' '}
              <a
                href={tosHref}
                target="_blank"
                rel="noopener noreferrer"
                className="text-brand hover:underline"
              >
                Terms of Service
              </a>{' '}
              and{' '}
              <a
                href={privacyHref}
                target="_blank"
                rel="noopener noreferrer"
                className="text-brand hover:underline"
              >
                Privacy Policy
              </a>
              .
            </p>
            <AuthSecondaryActionButton
              onClick={handleCancelConfirm}
              className="hover:text-brand flex items-center justify-center gap-1.5"
            >
              <LuArrowLeft className="h-3 w-3" />
              Back to edit username
            </AuthSecondaryActionButton>
          </div>
        </DialogContent>
      </Dialog>
    </AuthScreenLayout>
  )
}
