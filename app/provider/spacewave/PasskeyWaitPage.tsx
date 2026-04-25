import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
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
  loginWithEntityPem,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { setPendingDesktopPasskeyState } from './desktop-passkey-state.js'
import { base64ToBytes, unwrapPemWithPin } from './keypair-utils.js'
import {
  isPasskeyPrfPinWrapped,
  unwrapPemWithPasskeyPrf,
} from './passkey-prf.js'

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

  const loginWithPem = useCallback(
    async (pemPrivateKey: Uint8Array) => {
      if (!root) {
        throw new Error('Not connected to server')
      }
      const sessionIndex = await loginWithEntityPem(root, pemPrivateKey)
      navigate({ path: `/u/${sessionIndex}` })
    },
    [navigate, root],
  )

  useEffect(() => {
    if (!isDesktop || !root) return
    const controller = new AbortController()
    setState({ step: 'waiting' })
    setPin('')
    setPinError('')

    const run = async () => {
      try {
        const resp = await withSpacewaveProvider(
          root,
          async (spacewave) =>
            await spacewave.startDesktopPasskey({}, controller.signal),
          controller.signal,
        )
        if (controller.signal.aborted) return

        switch (resp.result?.case) {
          case 'linked': {
            const result = resp.result.value
            const encryptedBlob = result?.encryptedBlob ?? ''
            if (!encryptedBlob) {
              throw new Error('Desktop passkey did not return an entity key')
            }
            if (result?.prfCapable) {
              const authParams = result.authParams ?? ''
              const prfOutput = result.prfOutput ?? ''
              if (!prfOutput || !authParams) {
                throw new Error(
                  'Desktop passkey did not return PRF unwrap data',
                )
              }
              const unwrapped = await withSpacewaveProvider(
                root,
                (spacewave) =>
                  unwrapPemWithPasskeyPrf(
                    spacewave,
                    encryptedBlob,
                    authParams,
                    base64ToBytes(prfOutput),
                    controller.signal,
                  ),
                controller.signal,
              )
              if (isPasskeyPrfPinWrapped(authParams)) {
                setState({
                  step: 'pin_prompt',
                  encryptedBlob: new TextDecoder().decode(unwrapped),
                })
                return
              }
              setState({ step: 'logging_in' })
              await loginWithPem(unwrapped)
              return
            }
            if (result?.pinWrapped) {
              setState({ step: 'pin_prompt', encryptedBlob })
              return
            }
            setState({ step: 'logging_in' })
            await loginWithPem(base64ToBytes(encryptedBlob))
            return
          }
          case 'newAccount': {
            const result = resp.result.value
            setPendingDesktopPasskeyState({
              nonce: result?.nonce ?? '',
              username: result?.username ?? '',
              credentialJson: result?.credentialJson ?? '',
              prfCapable: !!result?.prfCapable,
              prfSalt: result?.prfSalt ?? '',
              prfOutput: result?.prfOutput ?? '',
            })
            navigate({ path: '/auth/passkey/confirm' })
            return
          }
          default:
            throw new Error('Desktop passkey did not return a result')
        }
      } catch (err) {
        if (controller.signal.aborted) return
        const message = getErrorMessage(err, 'Passkey sign-in failed')
        if (message.includes('abort') || message.includes('cancel')) return
        setState({ step: 'error', message })
      }
    }

    void run()
    return () => {
      controller.abort()
    }
  }, [loginWithPem, navigate, retryCount, root])

  const handleRetry = useCallback(() => {
    setRetryCount((c) => c + 1)
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
      await loginWithPem(pemBytes)
    } catch {
      setPinError('Incorrect PIN')
      setState({ step: 'pin_prompt', encryptedBlob: state.encryptedBlob })
    }
  }, [loginWithPem, pin, root, state])

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
              onClick={handleRetry}
              icon={<LuFingerprint className="h-4 w-4" />}
            >
              Open again
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
            type="password"
            value={pin}
            onChange={(e) => {
              setPin(e.target.value)
              setPinError('')
            }}
            autoFocus
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
              icon={<LuFingerprint className="h-4 w-4" />}
            >
              Continue
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        </div>
      </AuthScreenLayout>
    )
  }

  const statusMessage =
    state.step === 'logging_in' ?
      'Signing in...'
    : 'Complete the passkey step in your browser'

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground flex items-center gap-2 text-lg font-semibold">
            <LuFingerprint className="h-5 w-5" />
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
              icon={<LuFingerprint className="h-4 w-4" />}
            >
              Open again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        )}
      </AuthStatusPanel>
    </AuthScreenLayout>
  )
}
