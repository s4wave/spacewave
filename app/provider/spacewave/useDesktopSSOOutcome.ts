import { useCallback } from 'react'
import { isDesktop } from '@aptre/bldr'

import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import type { Root } from '@s4wave/sdk/root/root.js'

import {
  loginWithEntityPem,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { bytesToBase64 } from './keypair-utils.js'

export type DesktopSSOOutcome =
  | { kind: 'pin'; encryptedBlob: string; username: string }
  | { kind: 'session'; sessionIndex: number }
  | { kind: 'new-account'; email: string; nonce: string }

// useDesktopSSOOutcome owns the desktop SSO RPC and its cancelable login.
export function useDesktopSSOOutcome(
  root: Root | null | undefined,
  provider: string,
  retryCount: number,
  onLoginStart: () => void,
) {
  return usePromise(
    useCallback(
      (signal) => {
        void retryCount
        if (!isDesktop || !root || !provider) return undefined
        return (async (): Promise<DesktopSSOOutcome> => {
          const response = await withSpacewaveProvider(
            root,
            (spacewave) =>
              spacewave.startDesktopSSO({ ssoProvider: provider }, signal),
            signal,
          )
          signal.throwIfAborted()

          switch (response.result?.case) {
            case 'linked': {
              const result = response.result.value
              const pemPrivateKey = result?.pemPrivateKey
              if (!pemPrivateKey || pemPrivateKey.length === 0) {
                throw new Error('Desktop SSO did not return an entity key')
              }
              if (result?.pinWrapped) {
                return {
                  kind: 'pin',
                  encryptedBlob: bytesToBase64(pemPrivateKey),
                  username: result.username ?? '',
                }
              }
              onLoginStart()
              return {
                kind: 'session',
                sessionIndex: await loginWithEntityPem(
                  root,
                  pemPrivateKey,
                  signal,
                ),
              }
            }
            case 'newAccount': {
              const result = response.result.value
              return {
                kind: 'new-account',
                email: result?.email ?? '',
                nonce: result?.nonce ?? '',
              }
            }
            default:
              throw new Error('Desktop SSO did not return a result')
          }
        })()
      },
      [onLoginStart, provider, retryCount, root],
    ),
  )
}
