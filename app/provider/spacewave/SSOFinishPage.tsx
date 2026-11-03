import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuCheck, LuCircleAlert } from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useParams, useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SSOCodeExchangeResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { HandoffRequest } from '@s4wave/core/session/handoff/handoff.pb.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { HandoffComplete } from '@s4wave/app/auth/HandoffComplete.js'
import {
  completeStoredHandoff,
  getAuthReturnPath,
} from '@s4wave/app/auth/handoff-state.js'
import { setPendingSSOState } from './sso-state.js'
import {
  clearSSOBrowserBinding,
  getSSOBrowserBinding,
} from './sso-start-intent.js'
import { base64ToBytes, unwrapPemWithPin } from './keypair-utils.js'
import { getErrorMessage, withSpacewaveProvider } from './auth-flow-shared.js'
import { SSOUnlockCard } from './SSOUnlockCard.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'relative',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    textAlign: 'center',
    zIndex: 10,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    maxWidth: 'var(--container-sm)',
  },
  rule5: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'underline',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

// SSOFinishState tracks the page lifecycle.
type SSOFinishState =
  | { step: 'exchanging' }
  | { step: 'linked'; result: SSOCodeExchangeResponse }
  | { step: 'pin_prompt'; result: SSOCodeExchangeResponse }
  | { step: 'logging_in' }
  | { step: 'complete' }
  | { step: 'handoff_complete'; request: HandoffRequest; sessionIndex: number }
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
  const [binding] = useState(getSSOBrowserBinding)
  const [pin, setPin] = useState('')
  const [pinError, setPinError] = useState('')
  const linkedResult = state.step === 'linked' ? state.result : null
  const pinPromptResult = state.step === 'pin_prompt' ? state.result : null

  const exchangeResult = usePromise(
    useCallback(
      (signal) => {
        if (!root || state.step !== 'exchanging') return undefined
        if (!nonce) return Promise.reject(new Error('Missing SSO nonce'))
        if (!binding) {
          return Promise.reject(
            new Error(
              'This sign-in link requires the browser state created when sign-in started',
            ),
          )
        }
        return withSpacewaveProvider(
          root,
          (spacewave) =>
            spacewave.ssoNonceExchange(
              {
                nonce,
                verifier: base64ToBytes(binding.verifier),
                devicePrivateKey: base64ToBytes(binding.devicePrivateKey),
              },
              signal,
            ),
          signal,
        )
      },
      [binding, nonce, root, state.step],
    ),
  )

  // Route the provider-owned exchange result into the page flow.
  useEffect(() => {
    if (state.step !== 'exchanging') return
    let cancelled = false
    queueMicrotask(() => {
      if (cancelled) return
      if (exchangeResult.error) {
        setState({
          step: 'error',
          message: `${getErrorMessage(
            exchangeResult.error,
            'SSO nonce expired, consumed, or invalid',
          )}. Start sign-in again; a successful response cannot be replayed.`,
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
      clearSSOBrowserBinding()
      navigate({ path: `/auth/sso/${provider}/confirm` })
    })
    return () => {
      cancelled = true
    }
  }, [exchangeResult.data, exchangeResult.error, nonce, navigate, state.step])

  // Existing user: decrypt blob and login with entity key.
  useEffect(() => {
    if (!linkedResult || !root) return
    const controller = new AbortController()
    let cancelled = false
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

        // eslint-disable-next-line react-doctor/async-defer-await -- login response handling must ignore results only after the RPC settles.
        const loginResp = await withSpacewaveProvider(
          root,
          (spacewave) =>
            spacewave.loginWithEntityKey(pemBytes, controller.signal),
          controller.signal,
        )
        if (cancelled) return

        const sessionIndex = loginResp.sessionListEntry?.sessionIndex ?? 0
        const handoff = await completeStoredHandoff(root, sessionIndex)
        if (cancelled) return
        if (handoff) {
          clearSSOBrowserBinding()
          setState({ step: 'handoff_complete', request: handoff, sessionIndex })
          return
        }
        clearSSOBrowserBinding()
        navigate({ path: `/u/${sessionIndex}` })
      } catch (e) {
        if (!cancelled) {
          setState({
            step: 'error',
            message: e instanceof Error ? e.message : 'Login failed',
          })
        }
      }
    }
    void run()
    return () => {
      cancelled = true
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

      const handoff = await completeStoredHandoff(root, sessionIndex)
      if (handoff) {
        clearSSOBrowserBinding()
        setState({ step: 'handoff_complete', request: handoff, sessionIndex })
        return
      }
      clearSSOBrowserBinding()
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
    navigate({ path: getAuthReturnPath() })
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

  if (state.step === 'handoff_complete') {
    return (
      <HandoffComplete
        request={state.request}
        sessionIndex={state.sessionIndex}
      />
    )
  }

  // Error state.
  if (state.step === 'error') {
    return (
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <LuCircleAlert sx={stylexStyles.rule1} />
          <h2 sx={stylexStyles2.rule3}>Sign-in failed</h2>
          <p sx={stylexStyles2.rule4}>{state.message}</p>
          <button
            type="button"
            onClick={() => {
              clearSSOBrowserBinding()
              navigate({ path: getAuthReturnPath() })
            }}
            sx={stylexStyles2.rule5}
          >
            Start sign-in again
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
            <h2 sx={stylexStyles2.rule3}>Welcome back</h2>
            <p sx={stylexStyles2.rule6}>Enter your PIN to finish signing in</p>
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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <AnimatedLogo followMouse={false} />
        {state.step === 'complete' ? (
          <LuCheck sx={stylexStyles.rule2} />
        ) : (
          <Spinner size="md" variant="muted" />
        )}
        <p sx={stylexStyles2.rule6}>{statusMessage}</p>
      </div>
    </div>
  )
}
