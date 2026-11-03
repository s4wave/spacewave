/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useCallback, useMemo, useState } from 'react'
import { LuMonitor, LuTerminal } from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { LoginForm, type LoginResult } from '@s4wave/web/ui/login-form.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import { setSSOStartIntent } from '@s4wave/app/provider/spacewave/sso-start-intent.js'
import { HandoffComplete, HandoffDetails } from './HandoffComplete.js'
import {
  clearStoredHandoffPayload,
  clientTypeLabel,
  decodeHandoffRequest,
  enrollHandoffSession,
  setStoredHandoffPayload,
} from './handoff-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
  },
  rule2: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'relative',
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

interface HandoffRouteHints {
  authIntent: string
  username: string
}

function parseHandoffRouteHints(): HandoffRouteHints {
  const hash = window.location.hash
  const idx = hash.indexOf('?')
  if (idx === -1) {
    return { authIntent: '', username: '' }
  }
  const params = new URLSearchParams(hash.slice(idx + 1))
  return {
    authIntent: params.get('intent') ?? '',
    username: (params.get('username') ?? '').toLowerCase(),
  }
}

// HandoffState tracks the handoff page lifecycle.
type HandoffState =
  | { step: 'auth' }
  | { step: 'completing' }
  | { step: 'complete'; sessionIndex: number }

// ClientTypeIcon renders the icon for the client type.
function ClientTypeIcon({ clientType }: { clientType: string }) {
  if (clientType === 'cli') {
    return <LuTerminal sx={stylexStyles.rule1} />
  }
  return <LuMonitor sx={stylexStyles.rule1} />
}

// HandoffPage handles browser-delegated auth for desktop/CLI clients.
// Route: #/auth/link/{base64url payload}
export function HandoffPage() {
  const params = useParams()
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const [state, setState] = useState<HandoffState>({ step: 'auth' })

  const handoffPayload = params.payload ?? ''
  const request = useMemo(
    () => decodeHandoffRequest(handoffPayload),
    [handoffPayload],
  )
  const routeHints = useMemo(() => parseHandoffRouteHints(), [])

  const label = useMemo(
    () => clientTypeLabel(request?.clientType ?? ''),
    [request?.clientType],
  )

  const handleLoginWithPassword = useCallback(
    async (
      username: string,
      password: string,
      turnstileToken: string,
    ): Promise<LoginResult> => {
      if (!root) {
        throw new Error('Not connected')
      }
      if (!request) {
        throw new Error('Invalid handoff request')
      }
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.loginAccount({
        entityId: username,
        turnstileToken,
        credential: {
          value: { password },
          case: 'password' as const,
        },
      })
      switch (resp.result?.case) {
        case 'session': {
          const idx = resp.result.value?.sessionIndex ?? 0
          setState({ step: 'completing' })
          await enrollHandoffSession(root, idx, request)
          clearStoredHandoffPayload()
          setState({ step: 'complete', sessionIndex: idx })
          return { type: 'session', sessionIndex: idx }
        }
        case 'isNewAccount':
          return { type: 'new_account' }
        case 'errorCode':
          return { type: 'error', errorCode: resp.result.value }
        default:
          return { type: 'error', errorCode: 'unknown' }
      }
    },
    [root, request],
  )

  const handleCreateAccountWithPassword = useCallback(
    async (
      username: string,
      password: string,
      turnstileToken: string,
    ): Promise<{ sessionIndex: number }> => {
      if (!root) {
        throw new Error('Not connected')
      }
      if (!request) {
        throw new Error('Invalid handoff request')
      }
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.createAccount({
        entityId: username,
        turnstileToken,
        credential: {
          value: { password },
          case: 'password' as const,
        },
      })
      const sessionIndex = resp.sessionListEntry?.sessionIndex ?? 0

      setState({ step: 'completing' })
      await enrollHandoffSession(root, sessionIndex, request)
      clearStoredHandoffPayload()
      setState({ step: 'complete', sessionIndex })

      return { sessionIndex }
    },
    [root, request],
  )

  const handleNavigateToSession = useCallback(() => {
    // In handoff mode, do not navigate to session.
    // The completion message is shown instead.
  }, [])

  const handleContinueWithPasskey = useCallback(() => {
    if (!handoffPayload) {
      throw new Error('Invalid handoff request')
    }
    setStoredHandoffPayload(handoffPayload)
    const usernameQuery =
      routeHints.username !== ''
        ? `?username=${encodeURIComponent(routeHints.username)}`
        : ''
    navigate({ path: `/auth/passkey${usernameQuery}` })
  }, [handoffPayload, navigate, routeHints.username])

  const handleLoginWithPem = useCallback(
    async (pemPrivateKey: Uint8Array): Promise<{ sessionIndex: number }> => {
      if (!root || !request) throw new Error('Invalid handoff request')
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.loginWithEntityKey(pemPrivateKey)
      const sessionIndex = resp.sessionListEntry?.sessionIndex ?? 0
      setState({ step: 'completing' })
      await enrollHandoffSession(root, sessionIndex, request)
      clearStoredHandoffPayload()
      setState({ step: 'complete', sessionIndex })
      return { sessionIndex }
    },
    [request, root],
  )

  const handleSignInWithSSO = useCallback(
    (provider: 'google' | 'github') => {
      if (!handoffPayload) throw new Error('Invalid handoff request')
      setStoredHandoffPayload(handoffPayload)
      setSSOStartIntent(provider, `/auth/link/${handoffPayload}`)
      navigate({ path: `/auth/sso/${provider}` })
    },
    [handoffPayload, navigate],
  )

  if (!request) {
    return (
      <div sx={stylexStyles2.rule1}>
        <p sx={stylexStyles2.rule2}>Invalid handoff link.</p>
      </div>
    )
  }

  if (state.step === 'completing') {
    return (
      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>
          <Spinner size="xl" variant="brand" />
          <h1 sx={stylexStyles2.rule5}>Completing sign-in…</h1>
          <p sx={stylexStyles2.rule6}>
            Sending credentials to Spacewave {label}.
          </p>
        </div>
      </div>
    )
  }

  if (state.step === 'complete') {
    return (
      <HandoffComplete request={request} sessionIndex={state.sessionIndex} />
    )
  }

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <div sx={stylexStyles2.rule7}>
            <ClientTypeIcon clientType={request.clientType ?? ''} />
            <h1 sx={stylexStyles2.rule5}>
              {routeHints.authIntent === 'signup'
                ? `Creating a Spacewave ${label} account`
                : `Signing in to Spacewave ${label}`}
            </h1>
          </div>
          <HandoffDetails
            username={
              routeHints.authIntent === 'signup' ? routeHints.username : ''
            }
            deviceName={request.deviceName ?? ''}
          />
        </>
      }
    >
      <LoginForm
        initialUsername={routeHints.username}
        cloudProviderConfig={cloudProviderConfig}
        onLoginWithPassword={handleLoginWithPassword}
        onCreateAccountWithPassword={handleCreateAccountWithPassword}
        onNavigateToSession={handleNavigateToSession}
        onContinueWithPasskey={handleContinueWithPasskey}
        onLoginWithPem={handleLoginWithPem}
        onSignInWithSSO={handleSignInWithSSO}
      />
    </AuthScreenLayout>
  )
}
