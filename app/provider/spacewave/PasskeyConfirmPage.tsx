import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuFingerprint,
  LuUserPlus,
} from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
  authInputClassName,
  getErrorMessage,
  isUsernameTakenError,
  loginWithEntityPem,
  normalizeUsernameInput,
  validateOptionalPin,
  validateUsername,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import {
  clearPendingDesktopPasskeyState,
  getPendingDesktopPasskeyState,
} from './desktop-passkey-state.js'
import {
  base64ToBytes,
  generateAuthKeypairs,
  wrapPemWithPin,
} from './keypair-utils.js'
import { OptionalPinLock } from './OptionalPinLock.js'
import { wrapPemWithPasskeyPrf } from './passkey-prf.js'

type PasskeyConfirmState =
  | { step: 'form' }
  | { step: 'creating' }
  | { step: 'logging_in' }
  | { step: 'error'; message: string }

const HIGHLIGHTS = ['End-to-end encrypted', 'Passkey-protected', 'Local-first']

// PasskeyConfirmPage completes new-account desktop passkey signup after browser registration.
// Route: /auth/passkey/confirm
export function PasskeyConfirmPage() {
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const pendingState = useMemo(() => getPendingDesktopPasskeyState(), [])
  const [state, setState] = useState<PasskeyConfirmState>({ step: 'form' })
  const [username, setUsername] = useState(pendingState?.username ?? '')
  const [usernameError, setUsernameError] = useState('')
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [pinError, setPinError] = useState('')

  const handleRestart = useCallback(() => {
    clearPendingDesktopPasskeyState()
    navigate({ path: '/auth/passkey/wait' })
  }, [navigate])

  const handleCancel = useCallback(() => {
    clearPendingDesktopPasskeyState()
    navigate({ path: '/login' })
  }, [navigate])

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

  const handleCreateAccount = useCallback(async () => {
    if (!pendingState) {
      setState({ step: 'error', message: 'Passkey session expired' })
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
    const wantsPin = pin.length > 0 || confirmPin.length > 0
    if (!root) {
      setState({ step: 'error', message: 'Not connected to server' })
      return
    }

    setState({ step: 'creating' })
    try {
      await withSpacewaveProvider(root, async (spacewave) => {
        const { entity, session } = await generateAuthKeypairs(spacewave)

        let wrappedEntityKey = entity.custodiedPemBase64
        let prfCapable = false
        let prfSalt = ''
        let authParams = ''
        const pinWrapped = wantsPin
        if (pendingState.prfCapable) {
          const plaintext =
            wantsPin ?
              await wrapPemWithPin(spacewave, entity.pem, pin)
            : entity.pem
          const prfWrapped = await wrapPemWithPasskeyPrf(
            spacewave,
            plaintext,
            base64ToBytes(pendingState.prfOutput),
            wantsPin,
          )
          wrappedEntityKey = prfWrapped.encryptedPrivkey
          prfCapable = true
          prfSalt = pendingState.prfSalt
          authParams = prfWrapped.authParams
        } else if (wantsPin) {
          wrappedEntityKey = await wrapPemWithPin(spacewave, entity.pem, pin)
        }

        await spacewave.confirmDesktopPasskey({
          nonce: pendingState.nonce,
          username,
          credentialJson: pendingState.credentialJson,
          wrappedEntityKey,
          entityPeerId: entity.peerId,
          sessionPeerId: session.peerId,
          pinWrapped,
          prfCapable,
          prfSalt,
          authParams,
        })

        setState({ step: 'logging_in' })
        const sessionIndex = await loginWithEntityPem(
          root,
          new TextEncoder().encode(entity.pem),
        )
        clearPendingDesktopPasskeyState()
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
  }, [confirmPin, navigate, pendingState, pin, root, username])

  if (!pendingState) {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Passkey session expired
            </h2>
          </>
        }
      >
        <AuthStatusPanel
          icon={<LuCircleAlert className="text-destructive h-10 w-10" />}
          message="Your desktop passkey session has expired. Start the passkey flow again."
        >
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRestart}
              icon={<LuFingerprint className="h-4 w-4" />}
            >
              Restart sign-in
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthStatusPanel>
      </AuthScreenLayout>
    )
  }

  if (state.step === 'error') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Passkey sign-in failed
            </h2>
          </>
        }
      >
        <AuthStatusPanel
          icon={<LuCircleAlert className="text-destructive h-10 w-10" />}
          message={state.message}
        >
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRestart}
              icon={<LuFingerprint className="h-4 w-4" />}
            >
              Restart sign-in
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthStatusPanel>
      </AuthScreenLayout>
    )
  }

  const isBusy = state.step === 'creating' || state.step === 'logging_in'
  const statusMessage =
    state.step === 'creating' ? 'Creating account...'
    : state.step === 'logging_in' ? 'Signing in...'
    : ''

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <div className="flex items-center gap-2">
            <div className="bg-brand/10 border-brand/30 flex h-11 w-11 items-center justify-center rounded-full border">
              <LuFingerprint className="text-brand h-5 w-5" />
            </div>
            <div className="text-left">
              <h2 className="text-foreground text-lg font-semibold">
                Finish passkey setup
              </h2>
              <p className="text-foreground-alt text-sm">
                Choose your Spacewave username to finish desktop sign-in.
              </p>
            </div>
          </div>
        </>
      }
    >
      <div className="flex w-full flex-col gap-6">
        <div className="grid gap-2 sm:grid-cols-3">
          {HIGHLIGHTS.map((item) => (
            <div
              key={item}
              className="border-foreground/10 bg-background/20 text-foreground-alt rounded-md border px-3 py-2 text-xs"
            >
              {item}
            </div>
          ))}
        </div>

        <AuthCard className="flex flex-col gap-6">
          <div className="flex w-full flex-col gap-2">
            <label
              htmlFor="desktop-passkey-username"
              className="text-foreground text-sm font-medium"
            >
              Username
            </label>
            <input
              id="desktop-passkey-username"
              type="text"
              value={username}
              onChange={handleUsernameChange}
              placeholder={pendingState.username || 'your-username'}
              autoFocus
              disabled={isBusy}
              className={cn(
                authInputClassName,
                usernameError && 'border-destructive',
              )}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !isBusy) {
                  void handleCreateAccount()
                }
              }}
            />
            {usernameError && (
              <p className="text-destructive text-xs">{usernameError}</p>
            )}
          </div>

          <OptionalPinLock
            pin={pin}
            confirmPin={confirmPin}
            pinError={pinError}
            onPinChange={handlePinChange}
            onConfirmPinChange={handleConfirmPinChange}
            onSubmit={() => void handleCreateAccount()}
            disabled={isBusy}
            pinInputId="desktop-passkey-pin"
          />

          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={() => void handleCreateAccount()}
              disabled={isBusy || !username || !!usernameError}
              icon={isBusy ? <Spinner /> : <LuUserPlus className="h-4 w-4" />}
            >
              {isBusy ? statusMessage : 'Create account'}
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthCard>
      </div>
    </AuthScreenLayout>
  )
}
