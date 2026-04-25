import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  recoverPasskeyEntityPem,
  recoverSSOEntityPem,
  resolveRecoveredEntityPem,
} from './accountEscalationUnlock.js'

const mockLookupProvider = vi.hoisted(() =>
  vi.fn<() => Promise<{ resourceRef: object; [Symbol.dispose]: () => void }>>(),
)
const mockPasskeyAuthOptions = vi.hoisted(() =>
  vi.fn<() => Promise<{ optionsJson: string }>>(),
)
const mockPasskeyAuthVerify = vi.hoisted(() =>
  vi.fn<
    () => Promise<{
      accountId?: string
      encryptedBlob?: string
      prfCapable?: boolean
      pinWrapped?: boolean
      authParams?: string
    }>
  >(),
)
const mockSsoCallback = vi.hoisted(() =>
  vi.fn<
    () => Promise<{
      linked?: boolean
      accountId?: string
      encryptedBlob?: string
      pinWrapped?: boolean
    }>
  >(),
)
const mockStartAuthentication = vi.hoisted(() =>
  vi.fn<() => Promise<{ clientExtensionResults: object }>>(),
)
const mockGetCredentialPrfOutput = vi.hoisted(() =>
  vi.fn<() => Uint8Array | null>(),
)
const mockIsPasskeyPrfPinWrapped = vi.hoisted(() => vi.fn<() => boolean>())
const mockUnwrapPemWithPasskeyPrf = vi.hoisted(() =>
  vi.fn<() => Promise<Uint8Array>>(),
)
const mockUnwrapPemWithPin = vi.hoisted(() =>
  vi.fn<() => Promise<Uint8Array>>(),
)

vi.mock('@simplewebauthn/browser', () => ({
  startAuthentication: () => mockStartAuthentication(),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    passkeyAuthOptions = mockPasskeyAuthOptions
    passkeyAuthVerify = mockPasskeyAuthVerify
    ssoCodeExchange = mockSsoCallback
  },
}))

vi.mock('@s4wave/app/provider/spacewave/passkey-prf.js', () => ({
  addAuthenticationPrfInputs: (v: unknown) => v,
  getCredentialPrfOutput: () => mockGetCredentialPrfOutput(),
  isPasskeyPrfPinWrapped: () => mockIsPasskeyPrfPinWrapped(),
  unwrapPemWithPasskeyPrf: () => mockUnwrapPemWithPasskeyPrf(),
}))

vi.mock('@s4wave/app/provider/spacewave/keypair-utils.js', () => ({
  base64ToBytes: (dat: string) =>
    Uint8Array.from(atob(dat), (c) => c.charCodeAt(0)),
  unwrapPemWithPin: () => mockUnwrapPemWithPin(),
}))

function makeRoot() {
  return {
    lookupProvider: mockLookupProvider,
  }
}

function makeProviderRef() {
  return {
    resourceRef: {},
    [Symbol.dispose]: () => {},
  }
}

describe('accountEscalationUnlock', () => {
  beforeEach(() => {
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockPasskeyAuthOptions.mockReset()
    mockPasskeyAuthVerify.mockReset()
    mockSsoCallback.mockReset()
    mockStartAuthentication.mockReset()
    mockGetCredentialPrfOutput.mockReset()
    mockIsPasskeyPrfPinWrapped.mockReset()
    mockUnwrapPemWithPasskeyPrf.mockReset()
    mockUnwrapPemWithPin.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('recovers a direct passkey PEM', async () => {
    mockPasskeyAuthOptions.mockResolvedValue({ optionsJson: '{}' })
    mockStartAuthentication.mockResolvedValue({ clientExtensionResults: {} })
    mockGetCredentialPrfOutput.mockReturnValue(null)
    mockPasskeyAuthVerify.mockResolvedValue({
      accountId: 'acct-1',
      encryptedBlob: btoa('pem-data'),
      prfCapable: false,
      pinWrapped: false,
    })

    const recovered = await recoverPasskeyEntityPem(makeRoot() as never, {
      expectedAccountId: 'acct-1',
    })
    expect(recovered).toEqual({
      case: 'pem',
      pemPrivateKey: Uint8Array.from(new TextEncoder().encode('pem-data')),
    })
  })

  it('returns a pin-wrapped passkey unlock when PRF unwrap still requires a pin', async () => {
    mockPasskeyAuthOptions.mockResolvedValue({ optionsJson: '{}' })
    mockStartAuthentication.mockResolvedValue({ clientExtensionResults: {} })
    mockGetCredentialPrfOutput.mockReturnValue(new Uint8Array([1, 2, 3]))
    mockPasskeyAuthVerify.mockResolvedValue({
      accountId: 'acct-1',
      encryptedBlob: 'ciphertext',
      prfCapable: true,
      authParams: 'auth-params',
    })
    mockUnwrapPemWithPasskeyPrf.mockResolvedValue(
      new TextEncoder().encode('wrapped-base64'),
    )
    mockIsPasskeyPrfPinWrapped.mockReturnValue(true)

    const recovered = await recoverPasskeyEntityPem(makeRoot() as never, {
      expectedAccountId: 'acct-1',
    })
    expect(recovered).toEqual({
      case: 'pin',
      encryptedBlobBase64: 'wrapped-base64',
    })
  })

  it('recovers a direct SSO PEM', async () => {
    mockSsoCallback.mockResolvedValue({
      linked: true,
      accountId: 'acct-2',
      encryptedBlob: btoa('sso-pem'),
      pinWrapped: false,
    })

    const recovered = await recoverSSOEntityPem(
      makeRoot() as never,
      'google',
      'oauth-code',
      'https://account.test/auth/sso/callback',
      'acct-2',
    )
    expect(recovered).toEqual({
      case: 'pem',
      pemPrivateKey: Uint8Array.from(new TextEncoder().encode('sso-pem')),
    })
  })

  it('uses the desktop reauth relay when a native session and target signer are provided', async () => {
    const startDesktopPasskeyReauth = vi.fn().mockResolvedValue({
      encryptedBlob: 'ciphertext',
      prfCapable: true,
      authParams: 'auth-params',
      prfOutput: btoa('\u0001\u0002\u0003'),
      pinWrapped: false,
    })
    mockUnwrapPemWithPasskeyPrf.mockResolvedValue(new Uint8Array([4, 5, 6]))
    mockIsPasskeyPrfPinWrapped.mockReturnValue(false)

    const recovered = await recoverPasskeyEntityPem(makeRoot() as never, {
      desktopSession: {
        startDesktopPasskeyReauth,
      },
      targetPeerId: 'peer-passkey',
    })

    expect(startDesktopPasskeyReauth).toHaveBeenCalledWith(
      { peerId: 'peer-passkey' },
      undefined,
    )
    expect(recovered).toEqual({
      case: 'pem',
      pemPrivateKey: new Uint8Array([4, 5, 6]),
    })
  })

  it('unwraps a pin-protected recovered blob', async () => {
    mockUnwrapPemWithPin.mockResolvedValue(new Uint8Array([7, 8, 9]))

    const pem = await resolveRecoveredEntityPem(
      makeRoot() as never,
      {
        case: 'pin',
        encryptedBlobBase64: 'wrapped',
      },
      '1234',
    )
    expect(pem).toEqual(new Uint8Array([7, 8, 9]))
    expect(mockUnwrapPemWithPin).toHaveBeenCalled()
  })
})
