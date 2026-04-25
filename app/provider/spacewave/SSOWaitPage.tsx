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
  loginWithEntityPem,
  ProviderIcon,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { bytesToBase64, unwrapPemWithPin } from './keypair-utils.js'
import { setPendingSSOState } from './sso-state.js'
import { SSOUnlockCard } from './SSOUnlockCard.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'

type SSOWaitState =
  | { step: 'waiting' }
  | { step: 'logging_in' }
  | { step: 'redirecting' }
  | {
      step: 'pin_prompt'
      encryptedBlob: string
      username: string
      email: string
    }
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

  useEffect(() => {
    if (!isDesktop || !root || !provider) return
    const controller = new AbortController()
    setState({ step: 'waiting' })

    const run = async () => {
      try {
        const resp = await withSpacewaveProvider(
          root,
          async (spacewave) =>
            await spacewave.startDesktopSSO(
              { ssoProvider: provider },
              controller.signal,
            ),
          controller.signal,
        )
        if (controller.signal.aborted) return

        switch (resp.result?.case) {
          case 'linked': {
            const result = resp.result.value
            const pemPrivateKey = result?.pemPrivateKey
            if (!pemPrivateKey || pemPrivateKey.length === 0) {
              throw new Error('Desktop SSO did not return an entity key')
            }
            if (result?.pinWrapped) {
              setState({
                step: 'pin_prompt',
                encryptedBlob: bytesToBase64(pemPrivateKey),
                username: result?.username ?? '',
                email: result?.email ?? '',
              })
              return
            }
            setState({ step: 'logging_in' })
            const sessionIndex = await loginWithEntityPem(root, pemPrivateKey)
            navigate({ path: `/u/${sessionIndex}` })
            return
          }
          case 'newAccount': {
            const result = resp.result.value
            setPendingSSOState({
              provider,
              email: result?.email ?? '',
              nonce: result?.nonce ?? '',
              isDesktop: true,
            })
            navigate({ path: `/auth/sso/${provider}/confirm` })
            return
          }
          default:
            throw new Error('Desktop SSO did not return a result')
        }
      } catch (err) {
        if (controller.signal.aborted) return
        const message = getErrorMessage(err, 'Sign-in failed')
        if (message.includes('abort') || message.includes('cancel')) return
        setState({ step: 'error', message })
      }
    }

    void run()
    return () => {
      controller.abort()
    }
  }, [navigate, provider, retryCount, root])

  useEffect(() => {
    if (isDesktop || !provider) return
    const ssoBaseUrl = cloudProviderConfig?.ssoBaseUrl
    if (!ssoBaseUrl) return
    setState({ step: 'redirecting' })
    const origin = encodeURIComponent(window.location.origin)
    window.location.assign(`${ssoBaseUrl}/${provider}?origin=${origin}`)
  }, [cloudProviderConfig, provider])

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
      const sessionIndex = await loginWithEntityPem(root, pemBytes)
      navigate({ path: `/u/${sessionIndex}` })
    } catch {
      setPinError('Incorrect PIN')
      setState({
        step: 'pin_prompt',
        encryptedBlob: state.encryptedBlob,
        username: state.username,
        email: state.email,
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
              icon={<ProviderIcon provider={provider} className="h-4 w-4" />}
            >
              Try again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              className="flex items-center justify-center gap-2"
            >
              <LuArrowLeft className="h-4 w-4" />
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
          email={state.email}
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
    state.step === 'logging_in' ? 'Signing in with your entity key.'
    : state.step === 'redirecting' ? `Redirecting to ${providerLabel}.`
    : isDesktop ? 'Finish sign-in in your browser, then return here.'
    : `Connecting to ${providerLabel}.`

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 className="text-foreground flex items-center gap-2 text-lg font-semibold">
            <ProviderIcon provider={provider} className="h-5 w-5" />
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
              icon={<ProviderIcon provider={provider} className="h-4 w-4" />}
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
      </div>
    </AuthScreenLayout>
  )
}
