import { useCallback, useEffect, useState } from 'react'
import { LuArrowLeft, LuCircleAlert, LuFingerprint } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import {
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
  authInputClassName,
  getErrorMessage,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { setPendingDesktopPasskeyState } from './desktop-passkey-state.js'
import { unwrapPemWithPin } from './keypair-utils.js'
import { useDesktopPasskeyOutcome } from './useDesktopPasskeyOutcome.js'

type PasskeyWaitState =
  | { step: 'waiting' }
  | { step: 'logging_in' }
  | { step: 'pin_prompt'; encryptedBlob: string }
  | { step: 'error'; message: string }

// PasskeyWaitPage starts the native desktop passkey flow and waits for the browser result.
// Route: /auth/passkey/wait
export function PasskeyWaitPage() {
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, setState] = useState<PasskeyWaitState>({ step: 'waiting' })
  const [retryCount, setRetryCount] = useState(0)
  const [pin, setPin] = useState('')
  const [pinError, setPinError] = useState('')
  const handlePinInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleLoginStart = useCallback(() => {
    setState({ step: 'logging_in' })
  }, [])
  const outcome = useDesktopPasskeyOutcome(root, retryCount, handleLoginStart)

  useEffect(() => {
    if (outcome.error) {
      const message = getErrorMessage(outcome.error, 'Passkey sign-in failed')
      if (message.includes('abort') || message.includes('cancel')) return
      setState({ step: 'error', message })
      return
    }
    if (!outcome.data) return

    if (outcome.data.kind === 'pin') {
      setState({
        step: 'pin_prompt',
        encryptedBlob: outcome.data.encryptedBlob,
      })
      return
    }
    if (outcome.data.kind === 'new-account') {
      const { kind: _, ...pendingState } = outcome.data
      setPendingDesktopPasskeyState(pendingState)
      navigate({ path: '/auth/passkey/confirm' })
      return
    }
    navigate({ path: `/u/${outcome.data.sessionIndex}` })
  }, [navigate, outcome.data, outcome.error])

  const handleRetry = useCallback(() => {
    setState({ step: 'waiting' })
    setPin('')
    setPinError('')
    setRetryCount((count) => count + 1)
  }, [])

  const handleCancel = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleSubmitPin = useCallback(async () => {
    if (state.step !== 'pin_prompt') return
    if (!root) {
      setPinError('Provider is not ready')
      return
    }
    if (!pin) {
      setPinError('Enter your PIN')
      return
    }
    setPinError('')
    setState({ step: 'logging_in' })
    try {
      const pemBytes = await withSpacewaveProvider(root, (spacewave) =>
        unwrapPemWithPin(spacewave, state.encryptedBlob, pin),
      )
      const sessionOutcome = await withSpacewaveProvider(root, (spacewave) =>
        spacewave.loginWithEntityKey(pemBytes),
      )
      navigate({
        path: `/u/${sessionOutcome.sessionListEntry?.sessionIndex ?? 0}`,
      })
    } catch {
      setPinError('Incorrect PIN')
      setState({ step: 'pin_prompt', encryptedBlob: state.encryptedBlob })
    }
  }, [navigate, pin, root, state])

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
              onClick={handleRetry}
              icon={<LuFingerprint className="size-4" />}
            >
              Open again
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

  if (state.step === 'pin_prompt') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Enter your PIN
            </h2>
          </>
        }
      >
        <div className="flex w-full flex-col gap-4">
          <p className="text-foreground-alt text-sm">
            This passkey protects a PIN-wrapped key. Enter the PIN to finish
            signing in.
          </p>
          <input
            ref={handlePinInputRef}
            type="password"
            value={pin}
            onChange={(e) => {
              setPin(e.target.value)
              setPinError('')
            }}
            className={authInputClassName}
            placeholder="PIN"
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                void handleSubmitPin()
              }
            }}
          />
          {pinError && <p className="text-destructive text-xs">{pinError}</p>}
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={() => void handleSubmitPin()}
              icon={<LuFingerprint className="size-4" />}
            >
              Continue
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        </div>
      </AuthScreenLayout>
    )
  }

  const statusMessage =
    state.step === 'logging_in'
      ? 'Signing in...'
      : 'Complete the passkey step in your browser'

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground flex items-center gap-2 text-lg font-semibold">
            <LuFingerprint className="size-5" />
            Signing in with Passkey
          </h2>
        </>
      }
    >
      <AuthStatusPanel
        icon={<Spinner size="lg" className="text-brand" />}
        message={statusMessage}
      >
        {state.step === 'waiting' && (
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={<LuFingerprint className="size-4" />}
            >
              Open again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        )}
      </AuthStatusPanel>
    </AuthScreenLayout>
  )
}
