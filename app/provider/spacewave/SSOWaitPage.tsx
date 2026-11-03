/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { HandoffComplete } from '@s4wave/app/auth/HandoffComplete.js'
import {
  completeStoredHandoff,
  getAuthReturnPath,
} from '@s4wave/app/auth/handoff-state.js'
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
import {
  consumeSSOStartIntent,
  setSSOBrowserBinding,
} from './sso-start-intent.js'
import { setPendingSSOState } from './sso-state.js'
import { SSOUnlockCard } from './SSOUnlockCard.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    display: 'flex',
    width: '100%',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

type SSOWaitState =
  | { step: 'waiting' }
  | { step: 'logging_in' }
  | { step: 'redirecting' }
  | { step: 'pin_prompt'; encryptedBlob: string; username: string }
  | { step: 'handoff_complete'; request: HandoffRequest; sessionIndex: number }
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
    let cancelled = false
    queueMicrotask(() => {
      if (!cancelled) setState({ step: 'waiting' })
    })

    const run = async () => {
      try {
        // eslint-disable-next-line react-doctor/async-defer-await -- the abort guard belongs after the desktop SSO RPC returns.
        const resp = await withSpacewaveProvider(
          root,
          async (spacewave) =>
            await spacewave.startDesktopSSO(
              { ssoProvider: provider },
              controller.signal,
            ),
          controller.signal,
        )
        if (cancelled) return

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
              })
              return
            }
            setState({ step: 'logging_in' })
            const sessionIndex = await loginWithEntityPem(root, pemPrivateKey)
            const handoff = await completeStoredHandoff(root, sessionIndex)
            if (cancelled) return
            if (handoff) {
              setState({
                step: 'handoff_complete',
                request: handoff,
                sessionIndex,
              })
              return
            }
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
        if (cancelled) return
        const message = getErrorMessage(err, 'Sign-in failed')
        if (message.includes('abort') || message.includes('cancel')) return
        setState({ step: 'error', message })
      }
    }

    void run()
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [navigate, provider, retryCount, root])

  useEffect(() => {
    if (isDesktop || !provider || !root) return
    const ssoBaseUrl = cloudProviderConfig?.ssoBaseUrl
    if (!ssoBaseUrl) return
    const intent = consumeSSOStartIntent(provider)
    if (!intent.authorized) {
      navigate({ path: intent.returnTo, replace: true })
      return
    }
    const controller = new AbortController()
    let cancelled = false
    const run = async () => {
      try {
        const binding = await withSpacewaveProvider(
          root,
          (spacewave) => spacewave.prepareBrowserSSO({}, controller.signal),
          controller.signal,
        )
        if (cancelled) return
        if (
          binding.verifier?.length !== 32 ||
          binding.verifierHash?.length !== 32 ||
          binding.devicePublicKey?.length !== 32 ||
          binding.devicePrivateKey?.length !== 32
        ) {
          throw new Error('Browser SSO binding is invalid')
        }
        const verifier = bytesToBase64(binding.verifier)
        const verifierHash = bytesToBase64(binding.verifierHash)
        const devicePublicKey = bytesToBase64(binding.devicePublicKey)
        const devicePrivateKey = bytesToBase64(binding.devicePrivateKey)
        setSSOBrowserBinding({
          verifier,
          verifierHash,
          devicePublicKey,
          devicePrivateKey,
        })
        queueMicrotask(() => {
          if (!cancelled) setState({ step: 'redirecting' })
        })
        const authorizeUrl = new URL(`${ssoBaseUrl}/${provider}`)
        authorizeUrl.searchParams.set('origin', window.location.origin)
        authorizeUrl.searchParams.set('verifier_hash', verifierHash)
        authorizeUrl.searchParams.set('device_public_key', devicePublicKey)
        window.location.replace(authorizeUrl.toString())
      } catch (err) {
        if (!cancelled) {
          setState({
            step: 'error',
            message: getErrorMessage(err, 'Sign-in failed'),
          })
        }
      }
    }
    void run()
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [cloudProviderConfig, navigate, provider, root])

  const handleRetry = useCallback(() => {
    setRetryCount((c) => c + 1)
  }, [])

  const handleCancel = useCallback(() => {
    navigate({ path: getAuthReturnPath() })
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
      const handoff = await completeStoredHandoff(root, sessionIndex)
      if (handoff) {
        setState({ step: 'handoff_complete', request: handoff, sessionIndex })
        return
      }
      navigate({ path: `/u/${sessionIndex}` })
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
      <SSOWaitError
        provider={provider}
        message={state.message}
        onRetry={handleRetry}
        onCancel={handleCancel}
      />
    )
  }

  if (state.step === 'handoff_complete') {
    return (
      <HandoffComplete
        request={state.request}
        sessionIndex={state.sessionIndex}
      />
    )
  }

  if (state.step === 'pin_prompt') {
    return (
      <AuthScreenLayout
        alwaysShowIntro
        intro={
          <>
            <AnimatedLogo followMouse={false} />
            <h2 sx={stylexStyles3.rule1}>Welcome back</h2>
            <p sx={stylexStyles3.rule2}>Enter your PIN to finish signing in</p>
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

  const detail = ssoWaitDetail(state.step, providerLabel)

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 sx={stylexStyles3.rule3}>
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
            Signing in with {providerLabel}
          </h2>
        </>
      }
    >
      <div sx={stylexStyles3.rule4}>
        <LoadingCard
          view={{
            state: 'active',
            title: `Connecting to ${providerLabel}`,
            detail,
          }}
        />
        {isDesktop && state.step === 'waiting' && (
          <div sx={stylexStyles3.rule5}>
            <AuthPrimaryActionButton
              onClick={handleRetry}
              icon={
                <ProviderIcon provider={provider} sx={stylexStyles2.rule2} />
              }
            >
              Open again
            </AuthPrimaryActionButton>
            <AuthSecondaryActionButton
              onClick={handleCancel}
              sx={stylexStyles3.rule6}
            >
              <LuArrowLeft sx={stylexStyles.rule1} />
              Cancel
            </AuthSecondaryActionButton>
          </div>
        )}
      </div>
    </AuthScreenLayout>
  )
}

function SSOWaitError({
  provider,
  message,
  onRetry,
  onCancel,
}: {
  provider: string
  message: string
  onRetry: () => void
  onCancel: () => void
}) {
  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h2 sx={stylexStyles3.rule1}>Sign-in failed</h2>
        </>
      }
    >
      <div sx={stylexStyles3.rule4}>
        <LoadingCard
          view={{
            state: 'error',
            title: `Sign-in with ${getProviderLabel(provider)} failed`,
            error: message,
          }}
        />
        <div sx={stylexStyles3.rule5}>
          <AuthPrimaryActionButton
            onClick={isDesktop ? onRetry : onCancel}
            icon={<ProviderIcon provider={provider} sx={stylexStyles2.rule2} />}
          >
            {isDesktop ? 'Try again' : 'Return to sign-in'}
          </AuthPrimaryActionButton>
          <AuthSecondaryActionButton
            onClick={onCancel}
            sx={stylexStyles3.rule6}
          >
            <LuArrowLeft sx={stylexStyles.rule1} />
            Back to login
          </AuthSecondaryActionButton>
        </div>
      </div>
    </AuthScreenLayout>
  )
}

// ssoWaitDetail describes the current sign-in step.
function ssoWaitDetail(
  step: SSOWaitState['step'],
  providerLabel: string,
): string {
  return step === 'logging_in'
    ? 'Signing in with your entity key.'
    : step === 'redirecting'
      ? `Redirecting to ${providerLabel}.`
      : isDesktop
        ? 'Finish sign-in in your browser, then return here.'
        : `Connecting to ${providerLabel}.`
}
