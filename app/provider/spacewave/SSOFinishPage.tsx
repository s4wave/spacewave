import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuCheck, LuCircleAlert } from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useParams, useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SSOCodeExchangeResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { setPendingSSOState } from './sso-state.js'
import { base64ToBytes, unwrapPemWithPin } from './keypair-utils.js'
import { getErrorMessage, withSpacewaveProvider } from './auth-flow-shared.js'
import { SSOUnlockCard } from './SSOUnlockCard.js'

// SSOFinishState tracks the page lifecycle.
type SSOFinishState =
  | { step: 'exchanging' }
  | { step: 'linked'; result: SSOCodeExchangeResponse }
  | { step: 'pin_prompt'; result: SSOCodeExchangeResponse }
  | { step: 'logging_in' }
  | { step: 'complete' }
  | { step: 'error'; message: string }

// SSOFinishPage handles the OAuth redirect return.
// Route: #/auth/sso/finish/:nonce
// Exchanges the nonce for the OAuth result, then handles the linked (existing
// user) flow or redirects new users to /auth/sso/:provider/confirm.
export function SSOFinishPage() {
  const params = useParams()
  const nonce = params?.nonce ?? ''
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, setState] = useState<SSOFinishState>({ step: 'exchanging' })
  const [pin, setPin] = useState('')
  const [pinError, setPinError] = useState('')
  const linkedResult = state.step === 'linked' ? state.result : null
  const pinPromptResult = state.step === 'pin_prompt' ? state.result : null

  const exchangeResult = usePromise(
    useCallback(
      (signal) => {
        if (!nonce) return Promise.reject(new Error('Missing SSO nonce'))
        if (!root || state.step !== 'exchanging') return undefined
        return withSpacewaveProvider(
          root,
          (spacewave) => spacewave.ssoNonceExchange({ nonce }, signal),
          signal,
        )
      },
      [nonce, root, state.step],
    ),
  )

  // Route the provider-owned exchange result into the page flow.
  useEffect(() => {
    if (state.step !== 'exchanging') return
    if (exchangeResult.error) {
      setState({
        step: 'error',
        message: getErrorMessage(
          exchangeResult.error,
          'SSO nonce expired or invalid',
        ),
      })
      return
    }
    const result = exchangeResult.data
    if (!result) return
    if (result.linked) {
      setState({ step: 'linked', result })
      return
    }
    const provider = result.ssoProvider
    if (!provider) {
      setState({ step: 'error', message: 'SSO provider is missing' })
      return
    }
    setPendingSSOState({
      provider,
      email: result.email ?? '',
      nonce,
      isDesktop: false,
    })
    navigate({ path: `/auth/sso/${provider}/confirm` })
  }, [exchangeResult.data, exchangeResult.error, nonce, navigate, state.step])

  // Existing user: decrypt blob and login with entity key.
  useEffect(() => {
    if (!linkedResult || !root) return
    const controller = new AbortController()
    const run = async () => {
      try {
        const blob = linkedResult.encryptedBlob
        if (!blob) {
          setState({
            step: 'error',
            message: 'No encrypted key blob in SSO response',
          })
          return
        }

        if (linkedResult.pinWrapped) {
          setState({ step: 'pin_prompt', result: linkedResult })
          return
        }

        setState({ step: 'logging_in' })
        const pemBytes = base64ToBytes(blob)

        const loginResp = await withSpacewaveProvider(
          root,
          (spacewave) =>
            spacewave.loginWithEntityKey(pemBytes, controller.signal),
          controller.signal,
        )
        if (controller.signal.aborted) return

        const sessionIndex = loginResp.sessionListEntry?.sessionIndex ?? 0
        navigate({ path: `/u/${sessionIndex}` })
      } catch (e) {
        if (!controller.signal.aborted) {
          setState({
            step: 'error',
            message: e instanceof Error ? e.message : 'Login failed',
          })
        }
      }
    }
    void run()
    return () => {
      controller.abort()
    }
  }, [linkedResult, root, navigate])

  // PIN-wrapped: decrypt Layer 1 with PIN, then login.
  const handlePinSubmit = useCallback(async () => {
    if (!pinPromptResult) return
    if (!pin || !root) return

    const blob = pinPromptResult.encryptedBlob
    if (!blob) {
      setState({ step: 'error', message: 'No encrypted key blob' })
      return
    }

    setPinError('')
    setState({ step: 'logging_in' })

    try {
      const loginResp = await withSpacewaveProvider(root, async (spacewave) => {
        const pemBytes = await unwrapPemWithPin(spacewave, blob, pin)
        return await spacewave.loginWithEntityKey(pemBytes)
      })
      const sessionIndex = loginResp.sessionListEntry?.sessionIndex ?? 0

      setState({ step: 'complete' })
      navigate({ path: `/u/${sessionIndex}` })
    } catch {
      setPinError('Incorrect PIN')
      setState({ step: 'pin_prompt', result: pinPromptResult })
    }
  }, [pinPromptResult, pin, root, navigate])

  const handlePinChange = useCallback((value: string) => {
    setPin(value)
    setPinError('')
  }, [])

  const handleCancel = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  // Compute the status message.
  const statusMessage = useMemo(() => {
    switch (state.step) {
      case 'exchanging':
        return 'Verifying sign-in...'
      case 'linked':
        return 'Signing in...'
      case 'logging_in':
        return 'Mounting session...'
      case 'complete':
        return 'Welcome to Spacewave!'
      default:
        return ''
    }
  }, [state.step])

  // Error state.
  if (state.step === 'error') {
    return (
      <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center p-6">
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
        <div className="relative z-10 flex flex-col items-center gap-4 text-center">
          <LuCircleAlert className="text-destructive h-12 w-12" />
          <h2 className="text-foreground text-lg font-semibold">
            Sign-in failed
          </h2>
          <p className="text-foreground-alt max-w-sm text-sm">
            {state.message}
          </p>
          <button
            onClick={() => {
              navigate({ path: '/login' })
            }}
            className="text-brand hover:text-brand/80 mt-2 text-sm underline"
          >
            Back to login
          </button>
        </div>
      </div>
    )
  }

  // PIN prompt for PIN-wrapped keys.
  if (state.step === 'pin_prompt') {
    const provider = state.result.ssoProvider ?? ''
    const email = state.result.email ?? ''
    const username = state.result.username ?? ''
    return (
      <AuthScreenLayout
        alwaysShowIntro
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Welcome back
            </h2>
            <p className="text-foreground-alt text-sm">
              Enter your PIN to finish signing in
            </p>
          </>
        }
      >
        <SSOUnlockCard
          provider={provider}
          email={email}
          username={username}
          pin={pin}
          pinError={pinError}
          busy={false}
          onPinChange={handlePinChange}
          onSubmit={() => void handlePinSubmit()}
          onCancel={handleCancel}
        />
      </AuthScreenLayout>
    )
  }

  // Loading/progress states.
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
