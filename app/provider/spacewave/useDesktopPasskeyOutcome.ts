import { useCallback } from 'react'
import { isDesktop } from '@aptre/bldr'

import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import type { Root } from '@s4wave/sdk/root/root.js'

import {
  loginWithEntityPem,
  withSpacewaveProvider,
} from './auth-flow-shared.js'
import { base64ToBytes } from './keypair-utils.js'
import {
  isPasskeyPrfPinWrapped,
  unwrapPemWithPasskeyPrf,
} from './passkey-prf.js'

export type DesktopPasskeyOutcome =
  | { kind: 'pin'; encryptedBlob: string }
  | { kind: 'session'; sessionIndex: number }
  | {
      kind: 'new-account'
      nonce: string
      username: string
      credentialJson: string
      prfCapable: boolean
      prfSalt: string
      prfOutput: string
    }

// useDesktopPasskeyOutcome owns the desktop passkey RPC and its cancelable login.
export function useDesktopPasskeyOutcome(
  root: Root | null | undefined,
  retryCount: number,
  onLoginStart: () => void,
) {
  return usePromise(
    useCallback(
      (signal) => {
        void retryCount
        if (!isDesktop || !root) return undefined
        return (async (): Promise<DesktopPasskeyOutcome> => {
          const response = await withSpacewaveProvider(
            root,
            (spacewave) => spacewave.startDesktopPasskey({}, signal),
            signal,
          )
          signal.throwIfAborted()

          switch (response.result?.case) {
            case 'linked': {
              const result = response.result.value
              const encryptedBlob = result?.encryptedBlob ?? ''
              if (!encryptedBlob) {
                throw new Error('Desktop passkey did not return an entity key')
              }
              if (result?.prfCapable) {
                const authParams = result.authParams ?? ''
                const prfOutput = result.prfOutput ?? ''
                if (!prfOutput || !authParams) {
                  throw new Error(
                    'Desktop passkey did not return PRF unwrap data',
                  )
                }
                const unwrapped = await withSpacewaveProvider(
                  root,
                  (spacewave) =>
                    unwrapPemWithPasskeyPrf(
                      spacewave,
                      encryptedBlob,
                      authParams,
                      base64ToBytes(prfOutput),
                      signal,
                    ),
                  signal,
                )
                signal.throwIfAborted()
                if (isPasskeyPrfPinWrapped(authParams)) {
                  return {
                    kind: 'pin',
                    encryptedBlob: new TextDecoder().decode(unwrapped),
                  }
                }
                onLoginStart()
                return {
                  kind: 'session',
                  sessionIndex: await loginWithEntityPem(
                    root,
                    unwrapped,
                    signal,
                  ),
                }
              }
              if (result?.pinWrapped) {
                return { kind: 'pin', encryptedBlob }
              }
              onLoginStart()
              return {
                kind: 'session',
                sessionIndex: await loginWithEntityPem(
                  root,
                  base64ToBytes(encryptedBlob),
                  signal,
                ),
              }
            }
            case 'newAccount': {
              const result = response.result.value
              return {
                kind: 'new-account',
                nonce: result?.nonce ?? '',
                username: result?.username ?? '',
                credentialJson: result?.credentialJson ?? '',
                prfCapable: !!result?.prfCapable,
                prfSalt: result?.prfSalt ?? '',
                prfOutput: result?.prfOutput ?? '',
              }
            }
            default:
              throw new Error('Desktop passkey did not return a result')
          }
        })()
      },
      [onLoginStart, retryCount, root],
    ),
  )
}
