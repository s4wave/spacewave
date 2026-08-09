import { useCallback, useMemo, useState } from 'react'
import { startRegistration, startAuthentication } from '@simplewebauthn/browser'

import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  clearStoredHandoffPayload,
  completeStoredHandoff,
  hasStoredHandoffRequest,
} from '@s4wave/app/auth/handoff-state.js'
import { base64ToBytes, generateAuthKeypairs } from './keypair-utils.js'
import {
  getErrorMessage,
  isUsernameTakenError,
  loginWithEntityPem,
  normalizeUsernameInput,
  validateUsername,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { PasskeyChoice } from './PasskeyChoice.js'
import { PasskeyError } from './PasskeyError.js'
import { PasskeyProgress } from './PasskeyProgress.js'
import { PasskeyUsernameForm } from './PasskeyUsernameForm.js'
import {
  addAuthenticationPrfInputs,
  addRegistrationPrfInput,
  generatePasskeyPrfSalt,
  getCredentialPrfOutput,
  unwrapPemWithPasskeyPrf,
  wrapPemWithPasskeyPrf,
} from './passkey-prf.js'

type PasskeyState =
  | { step: 'username' }
  | { step: 'checking' }
  | { step: 'choice' }
  | { step: 'authenticating' }
  | { step: 'registering' }
  | { step: 'creating'; username: string }
  | { step: 'logging_in' }
  | { step: 'complete' }
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
  const [isHandoffFlow] = useState(() => hasStoredHandoffRequest())
  const [username, setUsername] = useState(() => getInitialPasskeyUsername())
  const [usernameError, setUsernameError] = useState('')
  const [choiceMessage, setChoiceMessage] = useState('')
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

        setState({ step: 'complete' })
        if (await completeStoredHandoff(root, sessionIndex)) {
          return
        }
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
    if (!root || !username) {
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

        setState({ step: 'complete' })
        if (await completeStoredHandoff(root, sessionIndex)) {
          return
        }
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
  }, [navigate, root, username])

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
        return isHandoffFlow ? 'Sign-in complete' : 'Welcome to Spacewave!'
      default:
        return ''
    }
  }, [isHandoffFlow, state.step])

  if (state.step === 'error') {
    return (
      <PasskeyError
        message={state.message}
        onRetry={() => {
          clearStoredHandoffPayload()
          setState({ step: 'username' })
          setUsernameError('')
        }}
        onBack={() => {
          clearStoredHandoffPayload()
          navigate({ path: '/login' })
        }}
      />
    )
  }

  if (state.step === 'choice') {
    return (
      <PasskeyChoice
        username={username}
        message={choiceMessage}
        onExistingPasskey={() => void handleExistingPasskey()}
        onCreateAccount={() => void handleCreateAccount()}
        onChangeUsername={() => {
          setChoiceMessage('')
          setState({ step: 'username' })
        }}
        onBack={() => navigate({ path: '/login' })}
      />
    )
  }

  if (state.step === 'username') {
    return (
      <PasskeyUsernameForm
        username={username}
        usernameError={usernameError}
        usernameInputRef={handleUsernameInputRef}
        onUsernameChange={handleUsernameChange}
        onContinue={() => void handleContinue()}
        onBack={() => navigate({ path: '/login' })}
      />
    )
  }

  return (
    <PasskeyProgress
      complete={state.step === 'complete'}
      message={statusMessage}
    />
  )
}
