import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuArrowLeft } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  getErrorMessage,
  getProviderLabel,
  ProviderIcon,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { unwrapPemWithPin } from './keypair-utils.js'
import { consumeSSOStartIntent } from './sso-start-intent.js'
import { setPendingSSOState } from './sso-state.js'
import { SSOUnlockCard } from './SSOUnlockCard.js'
import { useDesktopSSOOutcome } from './useDesktopSSOOutcome.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'

type SSOWaitState =
  | { step: 'waiting' }
  | { step: 'logging_in' }
  | { step: 'redirecting' }
  | { step: 'pin_prompt'; encryptedBlob: string; username: string }
  | { step: 'error'; message: string }

// SSOWaitPage handles the SSO in-progress state.
// Route: /auth/sso/:provider
// Desktop: starts the SSO RPC, shows waiting UI, handles result.
// Web: shows brief redirect message, then redirects to OAuth URL.
export function SSOWaitPage() {
  const params = useParams()
  const provider = params?.provider ?? ''
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const [state, setState] = useState<SSOWaitState>({ step: 'waiting' })
  const [retryCount, setRetryCount] = useState(0)
  const [pin, setPin] = useState('')
  const [pinError, setPinError] = useState('')
  const providerLabel = getProviderLabel(provider)

  const handleLoginStart = useCallback(() => {
    setState({ step: 'logging_in' })
  }, [])
  const outcome = useDesktopSSOOutcome(
    root,
    provider,
    retryCount,
    handleLoginStart,
  )

  useEffect(() => {
    if (outcome.error) {
      const message = getErrorMessage(outcome.error, 'Sign-in failed')
      if (message.includes('abort') || message.includes('cancel')) return
      setState({ step: 'error', message })
      return
    }
    if (!outcome.data) return

    if (outcome.data.kind === 'pin') {
      setState({
        step: 'pin_prompt',
        encryptedBlob: outcome.data.encryptedBlob,
        username: outcome.data.username,
      })
      return
    }
    if (outcome.data.kind === 'new-account') {
      setPendingSSOState({
        provider,
        email: outcome.data.email,
        nonce: outcome.data.nonce,
        isDesktop: true,
      })
      navigate({ path: `/auth/sso/${provider}/confirm` })
      return
    }
    navigate({ path: `/u/${outcome.data.sessionIndex}` })
  }, [navigate, outcome.data, outcome.error, provider])

  useEffect(() => {
    if (isDesktop || !provider) return
    const ssoBaseUrl = cloudProviderConfig?.ssoBaseUrl
    if (!ssoBaseUrl) return
    const intent = consumeSSOStartIntent(provider)
    if (!intent.authorized) {
      navigate({ path: intent.returnTo, replace: true })
      return
    }
    queueMicrotask(() => setState({ step: 'redirecting' }))
    const origin = encodeURIComponent(window.location.origin)
    window.location.replace(`${ssoBaseUrl}/${provider}?origin=${origin}`)
  }, [cloudProviderConfig, navigate, provider])

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
      setState({
        step: 'pin_prompt',
        encryptedBlob: state.encryptedBlob,
        username: state.username,
      })
    }
  }, [navigate, pin, root, state])

  const handlePinChange = useCallback((value: string) => {
    setPin(value)
    setPinError('')
  }, [])

  if (state.step === 'error') {
    return (
      <AuthScreenLayout
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 className="text-foreground text-lg font-semibold">
              Sign-in failed
            </h2>
          </>
        }
      >
        <div className="flex w-full flex-col items-center gap-4">
          <LoadingCard
            view={{
              state: 'error',
              title: `Sign-in with ${providerLabel} failed`,
              error: state.message,
            }}
          />
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={<ProviderIcon provider={provider} className="size-4" />}
            >
              Try again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="size-4" />
              Back to login
            </AuthSecondaryActionButton>
          </div>
        </div>
      </AuthScreenLayout>
    )
  }

  if (state.step === 'pin_prompt') {
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
          username={state.username}
          pin={pin}
          pinError={pinError}
          busy={false}
          onPinChange={handlePinChange}
          onSubmit={() => void handleSubmitPin()}
          onCancel={handleCancel}
        />
      </AuthScreenLayout>
    )
  }

  const detail =
    state.step === 'logging_in'
      ? 'Signing in with your entity key.'
      : state.step === 'redirecting'
        ? `Redirecting to ${providerLabel}.`
        : isDesktop
          ? 'Finish sign-in in your browser, then return here.'
          : `Connecting to ${providerLabel}.`

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground flex items-center gap-2 text-lg font-semibold">
            <ProviderIcon provider={provider} className="size-5" />
            Signing in with {providerLabel}
          </h2>
        </>
      }
    >
      <div className="flex w-full flex-col items-center gap-4">
        <LoadingCard
          view={{
            state: 'active',
            title: `Connecting to ${providerLabel}`,
            detail,
          }}
        />
        {isDesktop && state.step === 'waiting' && (
          <div className="flex w-full flex-col gap-2">
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={<ProviderIcon provider={provider} className="size-4" />}
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
      </div>
    </AuthScreenLayout>
  )
}
