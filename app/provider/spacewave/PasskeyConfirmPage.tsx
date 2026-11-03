/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuFingerprint,
  LuUserPlus,
} from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  AuthStatusPanel,
  authInputStyles,
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
import { TermsAgreement } from './TermsAgreement.js'
import { wrapPemWithPasskeyPrf } from './passkey-prf.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

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
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 11)`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule6: { textAlign: 'left' },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule9: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule10: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
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
    color: themeColors['--color-brand'],
  },
})

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
  const [agreed, setAgreed] = useState(false)

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
    if (!agreed) {
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
  }, [agreed, confirmPin, navigate, pendingState, pin, root, username])

  if (!pendingState) {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 sx={stylexStyles2.rule1}>Passkey session expired</h2>
          </>
        }
      >
        <AuthStatusPanel
          icon={<LuCircleAlert sx={stylexStyles.rule1} />}
          message="Your desktop passkey session has expired. Start the passkey flow again."
        >
          <div sx={stylexStyles2.rule2}>
            <AuthPrimaryActionButton
              onClick={handleRestart}
              icon={<LuFingerprint sx={stylexStyles.rule2} />}
            >
              Restart sign-in
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
              onClick={handleRestart}
              icon={<LuFingerprint sx={stylexStyles.rule2} />}
            >
              Restart sign-in
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

  const isBusy = state.step === 'creating' || state.step === 'logging_in'
  const statusMessage =
    state.step === 'creating'
      ? 'Creating account...'
      : state.step === 'logging_in'
        ? 'Signing in...'
        : ''

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <div sx={stylexStyles2.rule4}>
            <div sx={stylexStyles2.rule5}>
              <LuFingerprint sx={stylexStyles.rule3} />
            </div>
            <div sx={stylexStyles2.rule6}>
              <h2 sx={stylexStyles2.rule1}>Finish passkey setup</h2>
              <p sx={stylexStyles2.rule7}>
                Choose your Spacewave username to finish desktop sign-in.
              </p>
            </div>
          </div>
        </>
      }
    >
      <div sx={stylexStyles2.rule8}>
        <div sx={stylexStyles2.rule9}>
          {HIGHLIGHTS.map((item) => (
            <div key={item} sx={stylexStyles2.rule10}>
              {item}
            </div>
          ))}
        </div>

        <AuthCard sx={stylexStyles2.rule11}>
          <div sx={stylexStyles2.rule2}>
            <label htmlFor="desktop-passkey-username" sx={stylexStyles2.rule12}>
              Username
            </label>
            <input
              ref={handleUsernameInputRef}
              id="desktop-passkey-username"
              type="text"
              value={username}
              onChange={handleUsernameChange}
              placeholder={pendingState.username || 'your-username'}
              disabled={isBusy}
              sx={
                usernameError ? authInputStyles.invalid : authInputStyles.base
              }
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !isBusy) {
                  void handleCreateAccount()
                }
              }}
            />
            {usernameError && <p sx={stylexStyles2.rule13}>{usernameError}</p>}
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
            keyHeldWithoutPin={!pendingState.prfCapable}
          />

          <TermsAgreement
            agreed={agreed}
            onChange={setAgreed}
            disabled={isBusy}
          />

          <div sx={stylexStyles2.rule2}>
            <AuthPrimaryActionButton
              onClick={() => void handleCreateAccount()}
              disabled={isBusy || !username || !!usernameError || !agreed}
              icon={
                isBusy ? <Spinner /> : <LuUserPlus sx={stylexStyles.rule2} />
              }
            >
              {isBusy ? statusMessage : 'Create account'}
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              sx={stylexStyles2.rule3}
            >
              <LuArrowLeft sx={stylexStyles.rule2} />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </AuthCard>
      </div>
    </AuthScreenLayout>
  )
}
