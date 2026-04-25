import { describe, expect, it, vi } from 'vitest'

import type { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import {
  PasskeyPrfAuthParams,
  PasskeyPrfWrapAlgorithm,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import {
  addAuthenticationPrfInputs,
  addRegistrationPrfInput,
  generatePasskeyPrfSalt,
  getCredentialPrfOutput,
  isPasskeyPrfPinWrapped,
  unwrapPemWithPasskeyPrf,
  wrapPemWithPasskeyPrf,
} from './passkey-prf.js'

function uint8ArrayToBase64(dat: Uint8Array): string {
  return btoa(String.fromCharCode(...dat))
}

function testBytes(length: number): Uint8Array {
  return Uint8Array.from({ length }, (_, i) => i + 1)
}

describe('passkey-prf', () => {
  it('adds a registration PRF input as an ArrayBuffer', () => {
    const salt = 'AQIDBAUGBwgJCgsMDQ4PEA'
    const options = addRegistrationPrfInput({ challenge: 'abc' }, salt) as {
      extensions?: {
        prf?: {
          eval?: {
            first?: ArrayBuffer
          }
        }
      }
    }

    expect(options.extensions?.prf?.eval?.first).toBeInstanceOf(ArrayBuffer)
  })

  it('converts auth PRF inputs from base64url strings to buffers', () => {
    const salt = 'AQIDBAUGBwgJCgsMDQ4PEA'
    const options = addAuthenticationPrfInputs({
      challenge: 'abc',
      extensions: {
        prf: {
          evalByCredential: {
            'cred-1': {
              first: salt,
            },
          },
        },
      },
    }) as unknown as {
      extensions?: {
        prf?: {
          evalByCredential?: Record<string, { first: BufferSource }>
        }
      }
    }

    expect(
      options.extensions?.prf?.evalByCredential?.['cred-1']?.first,
    ).toBeInstanceOf(ArrayBuffer)
  })

  it('wraps and unwraps PEM with PRF output', async () => {
    const prfOutput = testBytes(32)
    const pem = '-----BEGIN TEST-----\nabc\n-----END TEST-----\n'
    const plaintext = new TextEncoder().encode(pem)
    const provider = {
      wrapWithPasskeyPrf: vi.fn(async () => ({
        encryptedBlobBase64: btoa('ciphertext'),
        authParamsBase64: btoa('auth-params'),
      })),
      unwrapWithPasskeyPrf: vi.fn(async () => ({
        plaintext,
        pinWrapped: false,
      })),
    } as unknown as SpacewaveProvider

    const wrapped = await wrapPemWithPasskeyPrf(provider, pem, prfOutput)
    const unwrapped = await unwrapPemWithPasskeyPrf(
      provider,
      wrapped.encryptedPrivkey,
      wrapped.authParams,
      prfOutput,
    )

    expect(provider.wrapWithPasskeyPrf).toHaveBeenCalledWith(
      {
        plaintext,
        prfOutput,
        pinWrapped: false,
      },
      undefined,
    )
    expect(provider.unwrapWithPasskeyPrf).toHaveBeenCalledWith(
      {
        encryptedBlobBase64: btoa('ciphertext'),
        authParamsBase64: btoa('auth-params'),
        prfOutput,
      },
      undefined,
    )
    expect(new TextDecoder().decode(unwrapped)).toBe(pem)
  })

  it('generates PRF salt through the provider', async () => {
    const provider = {
      generatePasskeyPrfSalt: vi.fn(async () => ({ prfSalt: 'salt-1' })),
    } as unknown as SpacewaveProvider

    await expect(generatePasskeyPrfSalt(provider)).resolves.toBe('salt-1')
    expect(provider.generatePasskeyPrfSalt).toHaveBeenCalledWith(undefined)
  })

  it('extracts the first PRF result from extension outputs', () => {
    const prfOutput = testBytes(32)
    const extracted = getCredentialPrfOutput({
      prf: {
        enabled: true,
        results: {
          first: prfOutput.buffer,
        },
      },
    })

    expect(extracted).toEqual(prfOutput)
  })

  it('fails to parse unsupported PRF algorithms', () => {
    const authParams = uint8ArrayToBase64(
      PasskeyPrfAuthParams.toBinary({
        algorithm: PasskeyPrfWrapAlgorithm.UNSPECIFIED,
        nonce: testBytes(12),
      }),
    )

    expect(() => isPasskeyPrfPinWrapped(authParams)).toThrow(
      'unsupported passkey PRF wrap algorithm',
    )
  })

  it('reads the generated PRF auth params pin flag', () => {
    const authParams = uint8ArrayToBase64(
      PasskeyPrfAuthParams.toBinary({
        algorithm: PasskeyPrfWrapAlgorithm.AES_256_GCM_V1,
        nonce: testBytes(12),
        pinWrapped: true,
      }),
    )

    expect(isPasskeyPrfPinWrapped(authParams)).toBe(true)
  })
})
