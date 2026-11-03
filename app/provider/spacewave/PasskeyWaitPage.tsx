/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuFingerprint,
} from '@s4wave/web/images/icons.js'

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
  authInputStyles,
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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule2: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

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
    let cancelled = false
    queueMicrotask(() => {
      if (cancelled) return
      setState({ step: 'waiting' })
      setPin('')
      setPinError('')
    })

    const run = async () => {
      try {
        // eslint-disable-next-line react-doctor/async-defer-await -- the abort guard belongs after the desktop passkey RPC returns.
        const resp = await withSpacewaveProvider(
          root,
          async (spacewave) =>
            await spacewave.startDesktopPasskey({}, controller.signal),
          controller.signal,
        )
        if (cancelled) return

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
        if (cancelled) return
        const message = getErrorMessage(err, 'Passkey sign-in failed')
        if (message.includes('abort') || message.includes('cancel')) return
        setState({ step: 'error', message })
      }
    }

    void run()
    return () => {
      cancelled = true
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
            <h2 sx={stylexStyles2.rule1}>Passkey sign-in failed</h2>
          </>
        }
      >
        <AuthStatusPanel
          icon={<LuCircleAlert sx={stylexStyles.rule1} />}
          message={state.message}
        >
          <div sx={stylexStyles2.rule2}>
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={<LuFingerprint sx={stylexStyles.rule2} />}
            >
              Open again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              sx={stylexStyles2.rule3}
            >
              <LuArrowLeft sx={stylexStyles.rule2} />
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
            <h2 sx={stylexStyles2.rule1}>Enter your PIN</h2>
          </>
        }
      >
        <div sx={stylexStyles2.rule4}>
          <p sx={stylexStyles2.rule5}>
            This passkey protects a PIN-wrapped key. Enter the PIN to finish
            signing in.
          </p>
          <input
            aria-label="PIN"
            ref={handlePinInputRef}
            type="password"
            value={pin}
            onChange={(e) => {
              setPin(e.target.value)
              setPinError('')
            }}
            sx={authInputStyles.base}
            placeholder="PIN"
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return
              if (e.key === 'Enter') {
                void handleSubmitPin()
              }
            }}
          />
          {pinError && <p sx={stylexStyles2.rule6}>{pinError}</p>}
          <div sx={stylexStyles2.rule2}>
            <AuthPrimaryActionButton
              onClick={() => void handleSubmitPin()}
              icon={<LuFingerprint sx={stylexStyles.rule2} />}
            >
              Continue
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              sx={stylexStyles2.rule3}
            >
              <LuArrowLeft sx={stylexStyles.rule2} />
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
          <h2 sx={stylexStyles2.rule7}>
            <LuFingerprint sx={stylexStyles.rule3} />
            Signing in with Passkey
          </h2>
        </>
      }
    >
      <AuthStatusPanel
        icon={<Spinner size="lg" variant="brand" />}
        message={statusMessage}
      >
        {state.step === 'waiting' && (
          <div sx={stylexStyles2.rule2}>
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={<LuFingerprint sx={stylexStyles.rule2} />}
            >
              Open again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              sx={stylexStyles2.rule3}
            >
              <LuArrowLeft sx={stylexStyles.rule2} />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        )}
      </AuthStatusPanel>
    </AuthScreenLayout>
  )
}
