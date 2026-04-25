import { base64URLStringToBuffer } from '@simplewebauthn/browser'

import type { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import {
  PasskeyPrfAuthParams,
  PasskeyPrfWrapAlgorithm,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

interface PasskeyPrfValuesJson {
  first: string
  second?: string
}

interface PasskeyPrfInputsJson {
  eval?: PasskeyPrfValuesJson
  evalByCredential?: Record<string, PasskeyPrfValuesJson>
}

interface PasskeyPrfOutputs {
  enabled?: boolean
  results?: {
    first: BufferSource
    second?: BufferSource
  }
}

interface PasskeyPrfClientExtensions {
  prf?: PasskeyPrfInputsJson
}

interface PasskeyPrfCredentialExtensions {
  prf?: PasskeyPrfOutputs
}

export interface PasskeyPrfWrapResult {
  authParams: string
  encryptedPrivkey: string
}

function base64ToUint8Array(dat: string): Uint8Array {
  return Uint8Array.from(atob(dat), (c) => c.charCodeAt(0))
}

function toUint8Array(dat: BufferSource): Uint8Array {
  if (dat instanceof ArrayBuffer) {
    return new Uint8Array(dat)
  }
  if (ArrayBuffer.isView(dat)) {
    return new Uint8Array(dat.buffer, dat.byteOffset, dat.byteLength)
  }
  return new Uint8Array(dat)
}

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function parseAuthParams(authParamsBase64: string) {
  const params = PasskeyPrfAuthParams.fromBinary(
    base64ToUint8Array(authParamsBase64),
  )
  if (params.algorithm !== PasskeyPrfWrapAlgorithm.AES_256_GCM_V1) {
    throw new Error('unsupported passkey PRF wrap algorithm')
  }
  return params
}

// isPasskeyPrfPinWrapped reports whether the PRF auth params require a PIN unwrap.
export function isPasskeyPrfPinWrapped(authParamsBase64: string): boolean {
  return parseAuthParams(authParamsBase64).pinWrapped ?? false
}

// generatePasskeyPrfSalt creates a provider-owned base64url PRF salt.
export async function generatePasskeyPrfSalt(
  spacewave: SpacewaveProvider,
  abortSignal?: AbortSignal,
): Promise<string> {
  const resp = await spacewave.generatePasskeyPrfSalt(abortSignal)
  const salt = resp.prfSalt ?? ''
  if (!salt) {
    throw new Error('Generated passkey PRF salt is empty')
  }
  return salt
}

// addRegistrationPrfInput injects a locally-generated PRF salt into registration options.
export function addRegistrationPrfInput<T>(options: T, prfSalt: string): T {
  const next = cloneJson(options) as Record<string, unknown>
  const extensions = (next.extensions ?? {}) as Record<string, unknown>
  extensions.prf = {
    eval: {
      first: base64URLStringToBuffer(prfSalt),
    },
  }
  next.extensions = extensions
  return next as T
}

// addAuthenticationPrfInputs converts server-provided PRF base64url fields into BufferSource inputs.
export function addAuthenticationPrfInputs<T>(options: T): T {
  const next = cloneJson(options) as Record<string, unknown>
  const extensions = next.extensions as PasskeyPrfClientExtensions | undefined
  const prf = extensions?.prf
  if (!prf) {
    return next as T
  }

  const converted: {
    eval?: { first: BufferSource; second?: BufferSource }
    evalByCredential?: Record<
      string,
      { first: BufferSource; second?: BufferSource }
    >
  } = {}

  if (prf.eval) {
    converted.eval = {
      first: base64URLStringToBuffer(prf.eval.first),
    }
    if (prf.eval.second) {
      converted.eval.second = base64URLStringToBuffer(prf.eval.second)
    }
  }
  if (prf.evalByCredential) {
    converted.evalByCredential = Object.fromEntries(
      Object.entries(prf.evalByCredential).map(([credentialId, values]) => {
        const convertedValues: { first: ArrayBuffer; second?: ArrayBuffer } = {
          first: base64URLStringToBuffer(values.first),
        }
        if (values.second) {
          convertedValues.second = base64URLStringToBuffer(values.second)
        }
        return [credentialId, convertedValues]
      }),
    )
  }

  next.extensions = {
    ...(next.extensions as Record<string, unknown> | undefined),
    prf: converted,
  }
  return next as T
}

// getCredentialPrfOutput extracts the first PRF output from a WebAuthn ceremony result.
export function getCredentialPrfOutput(
  clientExtensionResults: unknown,
): Uint8Array | null {
  const prf = (
    clientExtensionResults as PasskeyPrfCredentialExtensions | undefined
  )?.prf
  const first = prf?.results?.first
  if (!first) {
    return null
  }
  return toUint8Array(first)
}

// wrapPemWithPasskeyPrf encrypts PEM bytes with the provider PRF wrapper.
export async function wrapPemWithPasskeyPrf(
  spacewave: SpacewaveProvider,
  plaintext: string | Uint8Array,
  prfOutput: Uint8Array,
  pinWrapped = false,
  abortSignal?: AbortSignal,
): Promise<PasskeyPrfWrapResult> {
  const plaintextBytes =
    typeof plaintext === 'string' ?
      new TextEncoder().encode(plaintext)
    : plaintext
  const resp = await spacewave.wrapWithPasskeyPrf(
    {
      plaintext: plaintextBytes,
      prfOutput,
      pinWrapped,
    },
    abortSignal,
  )
  const authParams = resp.authParamsBase64 ?? ''
  const encryptedBlob = resp.encryptedBlobBase64 ?? ''
  if (!authParams || !encryptedBlob) {
    throw new Error('Passkey PRF wrap response is incomplete')
  }
  return {
    authParams,
    encryptedPrivkey: encryptedBlob,
  }
}

// unwrapPemWithPasskeyPrf decrypts a provider-encrypted PEM blob with PRF output.
export async function unwrapPemWithPasskeyPrf(
  spacewave: SpacewaveProvider,
  encryptedBlobBase64: string,
  authParamsBase64: string,
  prfOutput: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<Uint8Array> {
  const resp = await spacewave.unwrapWithPasskeyPrf(
    {
      encryptedBlobBase64,
      authParamsBase64,
      prfOutput,
    },
    abortSignal,
  )
  const plaintext = resp.plaintext
  if (!plaintext?.length) {
    throw new Error('Passkey PRF unwrap response is empty')
  }
  return plaintext
}
