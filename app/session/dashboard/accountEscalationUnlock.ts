import { startAuthentication } from '@simplewebauthn/browser'

import type { Root } from '@s4wave/sdk/root/root.js'
import type { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'

import { withSpacewaveProvider } from '@s4wave/app/provider/spacewave/auth-flow-shared.js'
import {
  addAuthenticationPrfInputs,
  getCredentialPrfOutput,
  isPasskeyPrfPinWrapped,
  unwrapPemWithPasskeyPrf,
} from '@s4wave/app/provider/spacewave/passkey-prf.js'
import {
  base64ToBytes,
  unwrapPemWithPin,
} from '@s4wave/app/provider/spacewave/keypair-utils.js'

export type RecoveredEntityPem =
  | { case: 'pem'; pemPrivateKey: Uint8Array }
  | { case: 'pin'; encryptedBlobBase64: string }

interface DesktopPasskeyReauthSession {
  startDesktopPasskeyReauth(
    request: { peerId: string },
    abortSignal?: AbortSignal,
  ): Promise<{
    encryptedBlob?: string
    prfCapable?: boolean
    prfSalt?: string
    authParams?: string
    pinWrapped?: boolean
    prfOutput?: string
  }>
}

export interface RecoverPasskeyEntityPemOptions {
  expectedAccountId?: string
  desktopSession?: DesktopPasskeyReauthSession
  targetPeerId?: string
  abortSignal?: AbortSignal
}

function requireMatchingAccount(
  expectedAccountId: string | undefined,
  actualAccountId: string | undefined,
): void {
  if (!expectedAccountId || !actualAccountId) {
    return
  }
  if (expectedAccountId !== actualAccountId) {
    throw new Error('The selected auth method belongs to a different account.')
  }
}

export async function recoverPasskeyEntityPem(
  root: Root,
  opts: RecoverPasskeyEntityPemOptions = {},
): Promise<RecoveredEntityPem> {
  async function recoverFromPasskeyArtifacts(
    spacewave: SpacewaveProvider,
    encryptedBlob: string,
    prfCapable: boolean,
    authParams: string,
    prfOutput: Uint8Array | null,
    pinWrapped: boolean,
  ): Promise<RecoveredEntityPem> {
    if (!encryptedBlob) {
      throw new Error('No encrypted key blob in passkey response')
    }
    if (prfCapable) {
      if (!authParams || !prfOutput) {
        throw new Error('Passkey PRF unwrap data was incomplete')
      }
      const unwrapped = await unwrapPemWithPasskeyPrf(
        spacewave,
        encryptedBlob,
        authParams,
        prfOutput,
      )
      if (isPasskeyPrfPinWrapped(authParams)) {
        return {
          case: 'pin',
          encryptedBlobBase64: new TextDecoder().decode(unwrapped),
        }
      }
      return {
        case: 'pem',
        pemPrivateKey: unwrapped,
      }
    }
    if (pinWrapped) {
      return {
        case: 'pin',
        encryptedBlobBase64: encryptedBlob,
      }
    }
    return {
      case: 'pem',
      pemPrivateKey: base64ToBytes(encryptedBlob),
    }
  }

  return await withSpacewaveProvider(root, async (spacewave) => {
    if (opts.desktopSession && opts.targetPeerId) {
      const reauth = await opts.desktopSession.startDesktopPasskeyReauth(
        { peerId: opts.targetPeerId },
        opts.abortSignal,
      )
      const prfOutput =
        reauth.prfCapable && reauth.prfOutput ?
          base64ToBytes(reauth.prfOutput)
        : null
      return await recoverFromPasskeyArtifacts(
        spacewave,
        reauth.encryptedBlob ?? '',
        reauth.prfCapable ?? false,
        reauth.authParams ?? '',
        prfOutput,
        reauth.pinWrapped ?? false,
      )
    }

    const optionsResp = await spacewave.passkeyAuthOptions({})
    if (!optionsResp.optionsJson) {
      throw new Error('Empty options from server')
    }
    const parsedOptions = addAuthenticationPrfInputs(
      JSON.parse(optionsResp.optionsJson) as Record<string, unknown>,
    )
    const options = parsedOptions as unknown as Parameters<
      typeof startAuthentication
    >[0]['optionsJSON']
    const credential = await startAuthentication({ optionsJSON: options })
    const prfOutput = getCredentialPrfOutput(credential.clientExtensionResults)
    const verifyResp = await spacewave.passkeyAuthVerify({
      credentialJson: JSON.stringify(credential),
    })
    requireMatchingAccount(opts.expectedAccountId, verifyResp.accountId)
    return await recoverFromPasskeyArtifacts(
      spacewave,
      verifyResp.encryptedBlob ?? '',
      verifyResp.prfCapable ?? false,
      verifyResp.authParams ?? '',
      prfOutput,
      verifyResp.pinWrapped ?? false,
    )
  })
}

export async function recoverSSOEntityPem(
  root: Root,
  provider: string,
  code: string,
  redirectUri: string,
  expectedAccountId?: string,
): Promise<RecoveredEntityPem> {
  return await withSpacewaveProvider(root, async (spacewave) => {
    const resp = await spacewave.ssoCodeExchange({
      provider,
      code,
      redirectUri,
    })
    if (!resp.linked) {
      throw new Error('That identity is not linked to an existing account.')
    }
    requireMatchingAccount(expectedAccountId, resp.accountId)
    const blob = resp.encryptedBlob ?? ''
    if (!blob) {
      throw new Error('No encrypted key blob in SSO response')
    }
    if (resp.pinWrapped) {
      return {
        case: 'pin',
        encryptedBlobBase64: blob,
      }
    }
    return {
      case: 'pem',
      pemPrivateKey: base64ToBytes(blob),
    }
  })
}

export async function resolveRecoveredEntityPem(
  root: Root,
  recovered: RecoveredEntityPem,
  pin?: string,
): Promise<Uint8Array> {
  if (recovered.case === 'pem') {
    return recovered.pemPrivateKey
  }
  if (!pin) {
    throw new Error('Enter your PIN')
  }
  return await withSpacewaveProvider(root, (spacewave) =>
    unwrapPemWithPin(spacewave, recovered.encryptedBlobBase64, pin),
  )
}
