/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuCheck,
  LuCircleAlert,
  LuFingerprint,
} from '@s4wave/web/images/icons.js'
import { startRegistration, startAuthentication } from '@simplewebauthn/browser'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { HandoffComplete } from '@s4wave/app/auth/HandoffComplete.js'
import {
  completeStoredHandoff,
  getAuthReturnPath,
} from '@s4wave/app/auth/handoff-state.js'
import { base64ToBytes, generateAuthKeypairs } from './keypair-utils.js'
import {
  authInputStyles,
  getErrorMessage,
  isUsernameTakenError,
  loginWithEntityPem,
  normalizeUsernameInput,
  validateUsername,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import {
  addAuthenticationPrfInputs,
  addRegistrationPrfInput,
  generatePasskeyPrfSalt,
  getCredentialPrfOutput,
  unwrapPemWithPasskeyPrf,
  wrapPemWithPasskeyPrf,
} from './passkey-prf.js'
import { TermsAgreement } from './TermsAgreement.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule3: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    maxWidth: 'var(--container-sm)',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    textDecorationLine: 'underline',
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule8: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule12: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    textDecorationLine: 'underline',
  },
  rule14: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule15: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule17: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule18: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule19: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-brand'],
  },
})

type PasskeyState =
  | { step: 'username' }
  | { step: 'checking' }
  | { step: 'choice' }
  | { step: 'authenticating' }
  | { step: 'registering' }
  | { step: 'creating'; username: string }
  | { step: 'logging_in' }
  | { step: 'complete' }
  | { step: 'handoff_complete'; request: HandoffRequest; sessionIndex: number }
  | { step: 'error'; message: string }

function getInitialPasskeyUsername(): string {
  const hash = window.location.hash
  const idx = hash.indexOf('?')
  if (idx === -1) {
    return ''
  }
  const params = new URLSearchParams(hash.slice(idx + 1))
  const next = normalizeUsernameInput(params.get('username') ?? '')
  return next.username
}

// PasskeyPage implements the web passkey sign-in and account creation flow.
export function PasskeyPage() {
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, setState] = useState<PasskeyState>({ step: 'username' })
  const [username, setUsername] = useState(() => getInitialPasskeyUsername())
  const [usernameError, setUsernameError] = useState('')
  const [choiceMessage, setChoiceMessage] = useState('')
  const [agreed, setAgreed] = useState(false)
  const handleBackToLogin = useCallback(() => {
    navigate({ path: getAuthReturnPath() })
  }, [navigate])
  const handleUsernameInputRef = useCallback(
    (node: HTMLInputElement | null) => {
      node?.focus()
    },
    [],
  )

  const handleUsernameChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = normalizeUsernameInput(e.target.value)
      setUsername(next.username)
      setUsernameError(next.error)
      setChoiceMessage('')
      if (state.step === 'choice') {
        setState({ step: 'username' })
      }
    },
    [state.step],
  )

  const handleExistingPasskey = useCallback(async () => {
    if (!root) {
      setState({ step: 'error', message: 'Not connected to server' })
      return
    }
    setState({ step: 'authenticating' })

    try {
      await withSpacewaveProvider(root, async (spacewave) => {
        const optionsResp = await spacewave.passkeyAuthOptions({ username })
        if (!optionsResp.optionsJson) {
          throw new Error('Empty options from server')
        }
        const parsedOptions = addAuthenticationPrfInputs(
          JSON.parse(optionsResp.optionsJson) as Record<string, unknown>,
        )
        const options = parsedOptions as unknown as Parameters<
          typeof startAuthentication
        >[0]['optionsJSON']

        const credential = await startAuthentication({ optionsJSON: options })
        const credentialJson = JSON.stringify(credential)
        const prfOutput = getCredentialPrfOutput(
          credential.clientExtensionResults,
        )
        const verifyResp = await spacewave.passkeyAuthVerify({
          credentialJson,
        })
        const blob = verifyResp.encryptedBlob ?? ''
        if (!blob) {
          throw new Error('No encrypted blob in response')
        }

        setState({ step: 'logging_in' })
        let pemBytes: Uint8Array
        if (verifyResp.prfCapable) {
          if (!verifyResp.authParams || !prfOutput) {
            throw new Error(
              'Passkey requires PRF output, but the browser did not return it',
            )
          }
          pemBytes = await unwrapPemWithPasskeyPrf(
            spacewave,
            blob,
            verifyResp.authParams,
            prfOutput,
          )
        } else {
          pemBytes = base64ToBytes(blob)
        }
        const sessionIndex = await loginWithEntityPem(root, pemBytes)

        const handoff = await completeStoredHandoff(root, sessionIndex)
        if (handoff) {
          setState({ step: 'handoff_complete', request: handoff, sessionIndex })
          return
        }
        setState({ step: 'complete' })
        navigate({ path: `/u/${sessionIndex}` })
      })
    } catch (e) {
      const msg = getErrorMessage(e, 'Authentication failed')
      if (msg.includes('NotAllowedError') || msg.includes('cancelled')) {
        setState({ step: 'choice' })
        return
      }
      setState({ step: 'error', message: msg })
    }
  }, [navigate, root, username])

  const handleContinue = useCallback(async () => {
    const usernameValidationError = validateUsername(username)
    if (usernameValidationError) {
      setUsernameError(usernameValidationError)
      return
    }
    if (!root) {
      setState({ step: 'error', message: 'Not connected to server' })
      return
    }

    setState({ step: 'checking' })
    setChoiceMessage('')

    try {
      await withSpacewaveProvider(root, async (spacewave) => {
        const result = await spacewave.passkeyCheckUsername({ username })
        if (!result.ok) {
          throw new Error('Passkey flow not available')
        }
        setState({ step: 'choice' })
      })
    } catch (e) {
      setState({
        step: 'error',
        message: getErrorMessage(e, 'Check failed'),
      })
    }
  }, [root, username])

  const handleCreateAccount = useCallback(async () => {
    if (!root || !username || !agreed) {
      return
    }
    setState({ step: 'registering' })

    try {
      await withSpacewaveProvider(root, async (spacewave) => {
        const chalResp = await spacewave.passkeyRegisterChallenge({ username })
        if (!chalResp.optionsJson) {
          throw new Error('Failed to get registration challenge')
        }
        const parsedRegOptions = JSON.parse(chalResp.optionsJson) as Record<
          string,
          unknown
        >
        const prfSalt = await generatePasskeyPrfSalt(spacewave)
        const regOptions = addRegistrationPrfInput(
          parsedRegOptions,
          prfSalt,
        ) as unknown as Parameters<typeof startRegistration>[0]['optionsJSON']

        const credential = await startRegistration({ optionsJSON: regOptions })
        const credentialJson = JSON.stringify(credential)
        const prfOutput = getCredentialPrfOutput(
          credential.clientExtensionResults,
        )
        setState({ step: 'creating', username })
        const { entity, session } = await generateAuthKeypairs(spacewave)
        const prfWrapped = prfOutput
          ? await wrapPemWithPasskeyPrf(spacewave, entity.pem, prfOutput)
          : null

        await spacewave.passkeyConfirmSignup({
          credentialJson,
          username,
          wrappedEntityKey:
            prfWrapped?.encryptedPrivkey ?? entity.custodiedPemBase64,
          entityPeerId: entity.peerId,
          sessionPeerId: session.peerId,
          pinWrapped: false,
          prfCapable: !!prfWrapped,
          prfSalt: prfWrapped ? prfSalt : '',
          authParams: prfWrapped?.authParams ?? '',
        })

        setState({ step: 'logging_in' })
        const sessionIndex = await loginWithEntityPem(
          root,
          new TextEncoder().encode(entity.pem),
        )

        const handoff = await completeStoredHandoff(root, sessionIndex)
        if (handoff) {
          setState({ step: 'handoff_complete', request: handoff, sessionIndex })
          return
        }
        setState({ step: 'complete' })
        navigate({ path: `/u/${sessionIndex}` })
      })
    } catch (e) {
      const msg = getErrorMessage(e, 'Account creation failed')
      if (msg.includes('NotAllowedError') || msg.includes('cancelled')) {
        setState({ step: 'choice' })
        return
      }
      if (isUsernameTakenError(e)) {
        setChoiceMessage(
          'That username is already taken. If this is your account and it does not have a passkey yet, sign in with another method and add one from account settings.',
        )
        setState({ step: 'choice' })
        return
      }
      setState({ step: 'error', message: msg })
    }
  }, [agreed, navigate, root, username])

  const statusMessage = useMemo(() => {
    switch (state.step) {
      case 'checking':
        return 'Preparing passkey flow...'
      case 'authenticating':
        return 'Waiting for passkey...'
      case 'registering':
        return 'Registering passkey...'
      case 'creating':
        return 'Creating account...'
      case 'logging_in':
        return 'Mounting session...'
      case 'complete':
        return 'Welcome to Spacewave!'
      default:
        return ''
    }
  }, [state.step])

  if (state.step === 'handoff_complete') {
    return (
      <HandoffComplete
        request={state.request}
        sessionIndex={state.sessionIndex}
      />
    )
  }

  if (state.step === 'error') {
    return (
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <LuCircleAlert sx={stylexStyles.rule1} />
          <h2 sx={stylexStyles2.rule3}>Passkey sign-in failed</h2>
          <p sx={stylexStyles2.rule4}>{state.message}</p>
          <button
            type="button"
            onClick={() => {
              setState({ step: 'username' })
              setUsernameError('')
            }}
            sx={stylexStyles2.rule5}
          >
            Try again
          </button>
          <button
            type="button"
            onClick={handleBackToLogin}
            sx={stylexStyles2.rule6}
          >
            Back to login
          </button>
        </div>
      </div>
    )
  }

  if (state.step === 'choice') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 sx={stylexStyles2.rule3}>Continue with passkey</h2>
            <p sx={stylexStyles2.rule4}>
              Choose how to continue for{' '}
              <span sx={stylexStyles2.rule7}>{username}</span>.
            </p>
          </>
        }
      >
        <div sx={stylexStyles2.rule8}>
          {choiceMessage && (
            <div sx={stylexStyles2.rule9}>
              <p sx={stylexStyles2.rule10}>{choiceMessage}</p>
            </div>
          )}
          <button
            type="button"
            onClick={() => void handleExistingPasskey()}
            sx={[stylexStyles2.rule11]}
          >
            <LuFingerprint sx={stylexStyles.rule2} />
            Sign in with Passkey
          </button>
          <TermsAgreement
            agreed={agreed}
            onChange={setAgreed}
            disabled={false}
          />
          <button
            type="button"
            onClick={() => void handleCreateAccount()}
            disabled={!agreed}
            sx={[stylexStyles2.rule12]}
          >
            <LuFingerprint sx={stylexStyles.rule2} />
            Create New Passkey Account
          </button>
          <p sx={stylexStyles2.rule10}>
            If your passkey cannot derive an encryption key, Spacewave Cloud
            keeps your account key so you can sign in, and we could use it to
            open your data.
          </p>
          <button
            type="button"
            onClick={() => {
              setChoiceMessage('')
              setState({ step: 'username' })
            }}
            sx={stylexStyles2.rule13}
          >
            Use a different username
          </button>
          <button
            type="button"
            onClick={handleBackToLogin}
            sx={stylexStyles2.rule6}
          >
            Back to login
          </button>
        </div>
      </AuthScreenLayout>
    )
  }

  if (state.step === 'username') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 sx={stylexStyles2.rule3}>Continue with passkey</h2>
          </>
        }
      >
        <div sx={stylexStyles2.rule14}>
          <div sx={stylexStyles2.rule15}>
            <label htmlFor="passkey-username" sx={stylexStyles2.rule16}>
              Username
            </label>
            <input
              ref={handleUsernameInputRef}
              id="passkey-username"
              type="text"
              value={username}
              onChange={handleUsernameChange}
              placeholder="your-username"
              autoComplete="username webauthn"
              sx={
                usernameError ? authInputStyles.invalid : authInputStyles.base
              }
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return
                if (e.key === 'Enter') {
                  void handleContinue()
                }
              }}
            />
            {usernameError && <p sx={stylexStyles2.rule17}>{usernameError}</p>}
          </div>

          <button
            type="button"
            onClick={() => void handleContinue()}
            disabled={!username || !!usernameError}
            sx={[stylexStyles2.rule18]}
          >
            <LuFingerprint sx={stylexStyles.rule2} />
            Continue
          </button>

          <button
            type="button"
            onClick={handleBackToLogin}
            sx={stylexStyles2.rule6}
          >
            Back to login
          </button>
        </div>
      </AuthScreenLayout>
    )
  }

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <AnimatedLogo followMouse={false} />
        {state.step === 'complete' ? (
          <LuCheck sx={stylexStyles.rule3} />
        ) : (
          <Spinner size="md" variant="muted" />
        )}
        <p sx={stylexStyles2.rule19}>{statusMessage}</p>
      </div>
    </div>
  )
}
