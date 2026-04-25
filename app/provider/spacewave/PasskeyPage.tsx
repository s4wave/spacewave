import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuCircleAlert, LuFingerprint } from 'react-icons/lu'
import { startRegistration, startAuthentication } from '@simplewebauthn/browser'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import {
  clearStoredHandoffPayload,
  completeStoredHandoff,
  hasStoredHandoffRequest,
} from '@s4wave/app/auth/handoff-state.js'
import { base64ToBytes, generateAuthKeypairs } from './keypair-utils.js'
import {
  authInputClassName,
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
        const verifyResp = await spacewave.passkeyAuthVerify({ credentialJson })
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
        const prfWrapped =
          prfOutput ?
            await wrapPemWithPasskeyPrf(spacewave, entity.pem, prfOutput)
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
      <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
        <div className="relative z-10 flex flex-col items-center gap-4 text-center">
          <LuCircleAlert className="text-destructive h-12 w-12" />
          <h2 className="text-foreground text-lg font-semibold">
            Passkey sign-in failed
          </h2>
          <p className="text-foreground-alt max-w-sm text-sm">
            {state.message}
          </p>
          <button
            onClick={() => {
              clearStoredHandoffPayload()
              setState({ step: 'username' })
              setUsernameError('')
            }}
            className="text-brand hover:text-brand/80 mt-2 text-sm underline"
          >
            Try again
          </button>
          <button
            onClick={() => {
              clearStoredHandoffPayload()
              navigate({ path: '/login' })
            }}
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
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
            <h2 className="text-foreground text-lg font-semibold">
              Continue with passkey
            </h2>
            <p className="text-foreground-alt max-w-sm text-sm">
              Choose how to continue for{' '}
              <span className="text-foreground font-medium">{username}</span>.
            </p>
          </>
        }
      >
        <div className="flex w-full flex-col gap-4">
          {choiceMessage && (
            <div className="border-warning/20 bg-warning/10 rounded-md border px-3 py-2">
              <p className="text-foreground-alt text-xs">{choiceMessage}</p>
            </div>
          )}
          <button
            onClick={() => void handleExistingPasskey()}
            className={cn(
              'flex w-full items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
              'bg-brand text-brand-foreground hover:bg-brand/90',
            )}
          >
            <LuFingerprint className="h-4 w-4" />
            Sign in with Passkey
          </button>
          <button
            onClick={() => void handleCreateAccount()}
            className={cn(
              'border-foreground/20 text-foreground hover:border-foreground/30 hover:bg-background/40',
              'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm font-medium transition-colors',
            )}
          >
            <LuFingerprint className="h-4 w-4" />
            Create New Passkey Account
          </button>
          <button
            onClick={() => {
              setChoiceMessage('')
              setState({ step: 'username' })
            }}
            className="text-brand hover:text-brand/80 text-sm underline"
          >
            Use a different username
          </button>
          <button
            onClick={() => navigate({ path: '/login' })}
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
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
            <h2 className="text-foreground text-lg font-semibold">
              Continue with passkey
            </h2>
          </>
        }
      >
        <div className="flex w-full flex-col gap-6">
          <div className="flex w-full flex-col gap-2">
            <label
              htmlFor="passkey-username"
              className="text-foreground text-sm font-medium"
            >
              Username
            </label>
            <input
              id="passkey-username"
              type="text"
              value={username}
              onChange={handleUsernameChange}
              placeholder="your-username"
              autoComplete="username webauthn"
              autoFocus
              className={cn(
                authInputClassName,
                usernameError && 'border-destructive',
              )}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  void handleContinue()
                }
              }}
            />
            {usernameError && (
              <p className="text-destructive text-xs">{usernameError}</p>
            )}
          </div>

          <button
            onClick={() => void handleContinue()}
            disabled={!username || !!usernameError}
            className={cn(
              'flex w-full items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
              'bg-brand text-brand-foreground hover:bg-brand/90',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            <LuFingerprint className="h-4 w-4" />
            Continue
          </button>

          <button
            onClick={() => navigate({ path: '/login' })}
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
          >
            Back to login
          </button>
        </div>
      </AuthScreenLayout>
    )
  }

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
      <div className="relative z-10 flex flex-col items-center gap-4 text-center">
        <AnimatedLogo followMouse={false} />
        {state.step === 'complete' ?
          <LuCheck className="text-brand h-6 w-6" />
        : <Spinner size="md" className="text-foreground-alt" />}
        <p className="text-foreground-alt text-sm">{statusMessage}</p>
      </div>
    </div>
  )
}
