import { useCallback } from 'react'
import { isDesktop } from '@aptre/bldr'

import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { CloudProviderConfig } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { LoginResult } from '@s4wave/web/ui/login-form.js'

// useCloudProviderConfig fetches the pre-auth cloud provider configuration.
export function useCloudProviderConfig(): CloudProviderConfig | null {
  const rootResource = RootContext.useContext()
  const providerResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (!root) return null
      const provider = cleanup(await root.lookupProvider('spacewave', signal))
      return cleanup(new SpacewaveProvider(provider.resourceRef))
    },
    [],
  )
  const cloudProviderConfigResource = useResource(
    providerResource,
    async (provider, signal) => {
      if (!provider) return null
      return await provider.getCloudProviderConfig(signal)
    },
    [],
  )
  return useResourceValue(cloudProviderConfigResource) ?? null
}

// useSpacewaveAuth provides auth callbacks for LoginForm that create a
// spacewave session. The navigateToSession callback controls where the
// user goes after successful auth (e.g. dashboard vs /plan/upgrade).
export function useSpacewaveAuth(
  navigateToSession: (sessionIndex: number, isNew: boolean) => void,
) {
  const rootResource = RootContext.useContext()
  const root = useResourceValue(rootResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const navigate = useNavigate()

  const handleLoginWithPassword = useCallback(
    async (
      username: string,
      password: string,
      turnstileToken: string,
    ): Promise<LoginResult> => {
      if (!root) throw new Error('Not connected')
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.loginAccount({
        entityId: username,
        turnstileToken,
        credential: { value: { password }, case: 'password' as const },
      })
      switch (resp.result?.case) {
        case 'session':
          return {
            type: 'session',
            sessionIndex: resp.result.value?.sessionIndex ?? 0,
          }
        case 'isNewAccount':
          return { type: 'new_account' }
        case 'errorCode':
          return { type: 'error', errorCode: resp.result.value }
        default:
          return { type: 'error', errorCode: 'unknown' }
      }
    },
    [root],
  )

  const handleCreateAccountWithPassword = useCallback(
    async (
      username: string,
      password: string,
      turnstileToken: string,
    ): Promise<{ sessionIndex: number }> => {
      if (!root) throw new Error('Not connected')
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.createAccount({
        entityId: username,
        turnstileToken,
        credential: { value: { password }, case: 'password' as const },
      })
      return { sessionIndex: resp.sessionListEntry?.sessionIndex ?? 0 }
    },
    [root],
  )

  const handleLoginWithPem = useCallback(
    async (pemPrivateKey: Uint8Array): Promise<{ sessionIndex: number }> => {
      if (!root) throw new Error('Not connected')
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.loginWithEntityKey(pemPrivateKey)
      return { sessionIndex: resp.sessionListEntry?.sessionIndex ?? 0 }
    },
    [root],
  )

  const startBrowserHandoff = useCallback(
    async (abortSignal?: AbortSignal): Promise<number> => {
      if (!root) throw new Error('Not connected')
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      const resp = await sw.startBrowserHandoff(
        {
          clientType: 'desktop',
        },
        abortSignal,
      )
      const sessionIndex = resp.sessionListEntry?.sessionIndex
      if (!sessionIndex) {
        throw new Error('Browser auth did not return a session')
      }
      return sessionIndex
    },
    [root],
  )

  // Passkey auth runs in the system browser on desktop and in-app on web.
  const handleContinueWithPasskey = useCallback(
    (abortSignal?: AbortSignal) => {
      void abortSignal
      if (isDesktop) {
        navigate({ path: '/auth/passkey/wait' })
        return
      }
      navigate({ path: '/auth/passkey' })
    },
    [navigate],
  )

  const handleSignInWithSSO = useCallback(
    (provider: 'google' | 'github') => {
      const providerEnabled =
        provider === 'google' ?
          !!cloudProviderConfig?.googleSsoEnabled
        : !!cloudProviderConfig?.githubSsoEnabled
      if (!providerEnabled) {
        throw new Error(`${provider} SSO is not configured`)
      }

      // Navigate to the SSO wait page which handles both desktop (RPC) and
      // web (OAuth redirect) flows.
      navigate({ path: `/auth/sso/${provider}` })
    },
    [cloudProviderConfig, navigate],
  )

  const handleContinueInBrowser = useCallback(
    async (abortSignal?: AbortSignal) => {
      navigateToSession(await startBrowserHandoff(abortSignal), false)
    },
    [navigateToSession, startBrowserHandoff],
  )

  return {
    cloudProviderConfig,
    handleLoginWithPassword,
    handleCreateAccountWithPassword,
    handleLoginWithPem,
    handleContinueInBrowser,
    handleContinueWithPasskey,
    handleSignInWithSSO,
  }
}
