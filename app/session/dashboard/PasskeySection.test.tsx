import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'

import { PasskeySection } from './PasskeySection.js'

const mockStartDesktopPasskeyRegisterHandoff = vi.hoisted(() => vi.fn())
const mockPasskeyRegisterVerify = vi.hoisted(() => vi.fn())
const mockPasskeyRegisterOptions = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', async () => {
  const actual = await vi.importActual<
    typeof import('@aptre/bldr-sdk/hooks/useResource.js')
  >('@aptre/bldr-sdk/hooks/useResource.js')
  return {
    ...actual,
    useResourceValue: () => ({
      lookupProvider: mockLookupProvider,
    }),
  }
})

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}
  },
}))

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@simplewebauthn/browser', () => ({
  startRegistration: vi.fn(),
}))

vi.mock('@s4wave/app/provider/spacewave/keypair-utils.js', () => ({
  base64ToBytes: (s: string) =>
    new Uint8Array(
      atob(s.replace(/-/g, '+').replace(/_/g, '/'))
        .split('')
        .map((c) => c.charCodeAt(0)),
    ),
  generateAuthKeypairs: () =>
    Promise.resolve({
      entity: {
        pem: 'PEM',
        peerId: 'peer-test',
        custodiedPemBase64: 'custodied-base64',
      },
      session: {
        peerId: 'session-peer',
      },
    }),
}))

vi.mock('@s4wave/app/provider/spacewave/passkey-prf.js', () => ({
  addRegistrationPrfInput: (opts: unknown) => opts,
  generatePasskeyPrfSalt: () => 'browser-salt',
  getCredentialPrfOutput: () => null,
  wrapPemWithPasskeyPrf: (
    _spacewave: unknown,
    _pem: string,
    _prfOutput: Uint8Array,
  ) =>
    Promise.resolve({
      encryptedPrivkey: 'encrypted-pem',
      authParams: 'auth-params',
    }),
}))

describe('PasskeySection desktop register branch', () => {
  const account = {
    value: {
      startDesktopPasskeyRegisterHandoff:
        mockStartDesktopPasskeyRegisterHandoff,
      passkeyRegisterVerify: mockPasskeyRegisterVerify,
      passkeyRegisterOptions: mockPasskeyRegisterOptions,
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  } as unknown as Resource<Account>

  beforeEach(() => {
    cleanup()
    mockStartDesktopPasskeyRegisterHandoff.mockReset()
    mockPasskeyRegisterVerify.mockReset()
    mockPasskeyRegisterOptions.mockReset()
    mockLookupProvider.mockResolvedValue({
      resourceRef: 'provider-ref',
      [Symbol.dispose]: vi.fn(),
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('calls the desktop register handoff and finalizes via passkeyRegisterVerify when PRF is present', async () => {
    mockStartDesktopPasskeyRegisterHandoff.mockResolvedValue({
      username: 'alice',
      credentialJson: '{"id":"cred-1"}',
      prfCapable: true,
      prfSalt: 'desktop-salt',
      prfOutput: btoa('desktop-output'),
    })
    mockPasskeyRegisterVerify.mockResolvedValue({ credentialId: 'cred-1' })

    render(
      <PasskeySection account={account} open={true} onOpenChange={() => {}} />,
    )

    fireEvent.click(screen.getByText('Start registration'))

    await waitFor(() => {
      expect(mockStartDesktopPasskeyRegisterHandoff).toHaveBeenCalledOnce()
    })
    expect(mockPasskeyRegisterOptions).not.toHaveBeenCalled()

    await waitFor(() => {
      expect(mockPasskeyRegisterVerify).toHaveBeenCalledOnce()
    })
    const verifyArgs = mockPasskeyRegisterVerify.mock.calls[0][0] as {
      credentialJson: string
      prfCapable: boolean
      prfSalt: string
      peerId: string
      authParams: string
      encryptedPrivkey: string
    }
    expect(verifyArgs.credentialJson).toBe('{"id":"cred-1"}')
    expect(verifyArgs.prfCapable).toBe(true)
    expect(verifyArgs.prfSalt).toBe('desktop-salt')
    expect(verifyArgs.peerId).toBe('peer-test')
    expect(verifyArgs.authParams).toBe('auth-params')
    expect(verifyArgs.encryptedPrivkey).toBe('encrypted-pem')

    await screen.findByText('Passkey registered')
  })

  it('falls back to custodied privkey when desktop handoff returns no PRF', async () => {
    mockStartDesktopPasskeyRegisterHandoff.mockResolvedValue({
      username: 'alice',
      credentialJson: '{"id":"cred-2"}',
      prfCapable: false,
      prfSalt: '',
      prfOutput: '',
    })
    mockPasskeyRegisterVerify.mockResolvedValue({ credentialId: 'cred-2' })

    render(
      <PasskeySection account={account} open={true} onOpenChange={() => {}} />,
    )

    fireEvent.click(screen.getByText('Start registration'))

    await waitFor(() => {
      expect(mockPasskeyRegisterVerify).toHaveBeenCalledOnce()
    })
    const verifyArgs = mockPasskeyRegisterVerify.mock.calls[0][0] as {
      prfCapable: boolean
      prfSalt: string
      encryptedPrivkey: string
      authParams: string
    }
    expect(verifyArgs.prfCapable).toBe(false)
    expect(verifyArgs.prfSalt).toBe('')
    expect(verifyArgs.authParams).toBe('')
    expect(verifyArgs.encryptedPrivkey).toBe('custodied-base64')
  })
})
