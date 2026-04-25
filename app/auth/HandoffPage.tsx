import { useCallback, useMemo, useState } from 'react'
import { LuMonitor, LuTerminal, LuCheck } from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { LoginForm, type LoginResult } from '@s4wave/web/ui/login-form.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import {
  decodeHandoffRequest,
  setStoredHandoffPayload,
} from './handoff-state.js'

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

// encryptForHandoffViaSession mounts the session and calls encryptForHandoff.
async function encryptForHandoffViaSession(
  root: Root,
  sessionIdx: number,
  devicePublicKey: Uint8Array | undefined,
  sessionNonce: string | undefined,
) {
  if (sessionIdx < 1) {
    throw new Error('Invalid session index')
  }
  const result = await root.mountSessionByIdx({ sessionIdx })
  if (!result) {
    throw new Error('Failed to mount session')
  }
  try {
    await result.session.spacewave.encryptForHandoff({
      devicePublicKey,
      sessionNonce,
    })
  } finally {
    result.session.release()
  }
}

// HandoffState tracks the handoff page lifecycle.
type HandoffState = 'auth' | 'completing' | 'complete'

// clientTypeLabel returns a display label for the client type.
function clientTypeLabel(clientType: string): string {
  switch (clientType) {
    case 'cli':
      return 'CLI'
    case 'desktop':
      return 'Desktop'
    default:
      return clientType || 'Desktop'
  }
}

// ClientTypeIcon renders the icon for the client type.
function ClientTypeIcon({ clientType }: { clientType: string }) {
  if (clientType === 'cli') {
    return <LuTerminal className="text-brand h-6 w-6" />
  }
  return <LuMonitor className="text-brand h-6 w-6" />
}

// HandoffPage handles browser-delegated auth for desktop/CLI clients.
// Route: #/auth/link/{base64url payload}
export function HandoffPage() {
  const params = useParams()
  const navigate = useNavigate()
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const [state, setState] = useState<HandoffState>('auth')

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
          setState('completing')
          await encryptForHandoffViaSession(
            root,
            idx,
            request.devicePublicKey,
            request.sessionNonce,
          )
          setState('complete')
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

      setState('completing')
      await encryptForHandoffViaSession(
        root,
        sessionIndex,
        request.devicePublicKey,
        request.sessionNonce,
      )
      setState('complete')

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
      routeHints.username !== '' ?
        `?username=${encodeURIComponent(routeHints.username)}`
      : ''
    navigate({ path: `/auth/passkey${usernameQuery}` })
  }, [handoffPayload, navigate, routeHints.username])

  if (!request) {
    return (
      <div className="bg-background-landing flex flex-1 flex-col items-center justify-center p-6">
        <p className="text-destructive text-sm">Invalid handoff link.</p>
      </div>
    )
  }

  if (state === 'completing') {
    return (
      <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 p-6">
        <ShootingStars className="pointer-events-none fixed inset-0" />
        <div className="relative z-10 flex flex-col items-center gap-4">
          <Spinner size="xl" className="text-brand" />
          <h1 className="text-xl font-bold tracking-wide">
            Completing sign-in...
          </h1>
          <p className="text-foreground-alt text-sm">
            Sending credentials to Spacewave {label}.
          </p>
        </div>
      </div>
    )
  }

  if (state === 'complete') {
    return (
      <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 p-6">
        <ShootingStars className="pointer-events-none fixed inset-0" />
        <div className="relative z-10 flex flex-col items-center gap-4">
          <div className="bg-brand/10 border-brand/30 flex h-16 w-16 items-center justify-center rounded-full border">
            <LuCheck className="text-brand h-8 w-8" />
          </div>
          <h1 className="text-xl font-bold tracking-wide">Sign-in complete</h1>
          <p className="text-foreground-alt text-sm">
            You can close this tab and return to Spacewave {label}.
          </p>
          {request.deviceName && (
            <p className="text-foreground-alt/60 text-xs">
              Device: {request.deviceName}
            </p>
          )}
        </div>
      </div>
    )
  }

  return (
    <AuthScreenLayout
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <div className="flex items-center gap-2">
            <ClientTypeIcon clientType={request.clientType ?? ''} />
            <h1 className="text-xl font-bold tracking-wide">
              {routeHints.authIntent === 'signup' ?
                `Creating a Spacewave ${label} account`
              : `Signing in to Spacewave ${label}`}
            </h1>
          </div>
          {routeHints.authIntent === 'signup' && routeHints.username && (
            <p className="text-foreground-alt max-w-sm text-center text-sm">
              Continue with passkey or use password to create{' '}
              <span className="text-foreground font-medium">
                {routeHints.username}
              </span>
              .
            </p>
          )}
          {request.deviceName && (
            <p className="text-foreground-alt/70 text-xs">
              Device: {request.deviceName}
            </p>
          )}
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
      />
    </AuthScreenLayout>
  )
}
