import { useCallback, useMemo, useState } from 'react'
import { LuArrowLeft, LuCircleAlert, LuFingerprint } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import {
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
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
import { wrapPemWithPasskeyPrf } from './passkey-prf.js'
import { PasskeyConfirmForm } from './PasskeyConfirmForm.js'

type PasskeyConfirmState =
  | { step: 'form' }
  | { step: 'creating' }
  | { step: 'logging_in' }
  | { step: 'error'; message: string }

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

  const handleUsernameInputRef = useCallback(
    (node: HTMLInputElement | null) => {
      node?.focus()
    },
    [],
  )

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
          const plaintext = wantsPin
            ? await wrapPemWithPin(spacewave, entity.pem, pin)
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
          icon={<LuCircleAlert className="text-destructive size-10" />}
          message="Your desktop passkey session has expired. Start the passkey flow again."
        >
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRestart}
              icon={<LuFingerprint className="size-4" />}
            >
              Restart sign-in
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
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
          icon={<LuCircleAlert className="text-destructive size-10" />}
          message={state.message}
        >
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRestart}
              icon={<LuFingerprint className="size-4" />}
            >
              Restart sign-in
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthStatusPanel>
      </AuthScreenLayout>
    )
  }

  const isBusy = state.step === 'creating' || state.step === 'logging_in'
  const statusMessage =
    state.step === 'creating'
      ? 'Creating account...'
      : state.step === 'logging_in'
        ? 'Signing in...'
        : ''

  return (
    <PasskeyConfirmForm
      username={username}
      placeholderUsername={pendingState.username}
      usernameError={usernameError}
      usernameInputRef={handleUsernameInputRef}
      pin={pin}
      confirmPin={confirmPin}
      pinError={pinError}
      isBusy={isBusy}
      statusMessage={statusMessage}
      onUsernameChange={handleUsernameChange}
      onPinChange={handlePinChange}
      onConfirmPinChange={handleConfirmPinChange}
      onCreateAccount={() => void handleCreateAccount()}
      onCancel={handleCancel}
    />
  )
}
