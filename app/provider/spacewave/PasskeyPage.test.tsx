import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PasskeyPage } from './PasskeyPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())
const mockPasskeyCheckUsername = vi.hoisted(() => vi.fn())
const mockPasskeyAuthOptions = vi.hoisted(() => vi.fn())
const mockPasskeyAuthVerify = vi.hoisted(() => vi.fn())
const mockPasskeyRegisterChallenge = vi.hoisted(() => vi.fn())
const mockPasskeyConfirmSignup = vi.hoisted(() => vi.fn())
const mockLoginWithEntityKey = vi.hoisted(() => vi.fn())
const mockStartAuthentication = vi.hoisted(() => vi.fn())
const mockStartRegistration = vi.hoisted(() => vi.fn())
const mockCompleteStoredHandoff = vi.hoisted(() => vi.fn())
const mockHasStoredHandoffRequest = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => ({
    lookupProvider: mockLookupProvider,
  }),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    passkeyCheckUsername = mockPasskeyCheckUsername
    passkeyAuthOptions = mockPasskeyAuthOptions
    passkeyAuthVerify = mockPasskeyAuthVerify
    passkeyRegisterChallenge = mockPasskeyRegisterChallenge
    passkeyConfirmSignup = mockPasskeyConfirmSignup
    loginWithEntityKey = mockLoginWithEntityKey
  },
}))

vi.mock('@simplewebauthn/browser', () => ({
  startAuthentication: mockStartAuthentication,
  startRegistration: mockStartRegistration,
}))

vi.mock('@s4wave/app/auth/handoff-state.js', () => ({
  clearStoredHandoffPayload: vi.fn(),
  completeStoredHandoff: (...args: [unknown, number]): Promise<boolean> =>
    mockCompleteStoredHandoff(...args) as Promise<boolean>,
  hasStoredHandoffRequest: (): boolean =>
    mockHasStoredHandoffRequest() as boolean,
}))

vi.mock('./keypair-utils.js', () => ({
  dnsLabelRegex: /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/,
  base64ToBytes: (dat: string) =>
    Uint8Array.from(atob(dat), (c) => c.charCodeAt(0)),
  generateAuthKeypairs: () =>
    Promise.resolve({
      entity: {
        pem: 'entity-pem',
        peerId: 'entity-peer-id',
        custodiedPemBase64: 'entity-base64',
      },
      session: {
        peerId: 'session-peer-id',
      },
    }),
}))

vi.mock('./passkey-prf.js', () => ({
  addAuthenticationPrfInputs: (v: unknown) => v,
  addRegistrationPrfInput: (v: unknown) => v,
  generatePasskeyPrfSalt: () => 'prf-salt',
  getCredentialPrfOutput: () => null,
  unwrapPemWithPasskeyPrf: vi.fn(),
  wrapPemWithPasskeyPrf: vi.fn(),
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div>logo</div>,
}))

vi.mock('@s4wave/app/auth/AuthScreenLayout.js', () => ({
  AuthScreenLayout: ({
    intro,
    children,
  }: {
    intro: React.ReactNode
    children: React.ReactNode
  }) => (
    <div>
      <div>{intro}</div>
      <div>{children}</div>
    </div>
  ),
}))

function makeProviderRef() {
  return {
    resourceRef: {},
    [Symbol.dispose]: () => {},
  }
}

describe('PasskeyPage', () => {
  beforeEach(() => {
    cleanup()
    window.location.hash = '#/auth/passkey'
    mockNavigate.mockReset()
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockPasskeyCheckUsername.mockReset()
    mockPasskeyAuthOptions.mockReset()
    mockPasskeyAuthVerify.mockReset()
    mockPasskeyRegisterChallenge.mockReset()
    mockPasskeyConfirmSignup.mockReset()
    mockLoginWithEntityKey.mockReset()
    mockStartAuthentication.mockReset()
    mockStartRegistration.mockReset()
    mockCompleteStoredHandoff.mockReset()
    mockHasStoredHandoffRequest.mockReset()
    mockCompleteStoredHandoff.mockResolvedValue(false)
    mockHasStoredHandoffRequest.mockReturnValue(false)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('keeps web passkey on stored handoff completion instead of desktop relay', async () => {
    mockHasStoredHandoffRequest.mockReturnValue(true)
    mockCompleteStoredHandoff.mockResolvedValue(true)
    mockPasskeyCheckUsername.mockResolvedValue({ ok: true })
    mockPasskeyAuthOptions.mockResolvedValue({ optionsJson: '{}' })
    mockStartAuthentication.mockResolvedValue({
      id: 'cred-3',
      clientExtensionResults: {},
    })
    mockPasskeyAuthVerify.mockResolvedValue({
      encryptedBlob: 'ZW50aXR5',
      prfCapable: false,
      pinWrapped: false,
    })
    mockLoginWithEntityKey.mockResolvedValue({
      sessionListEntry: { sessionIndex: 5 },
    })

    render(<PasskeyPage />)

    fireEvent.change(screen.getByPlaceholderText('your-username'), {
      target: { value: 'web-user' },
    })
    fireEvent.click(screen.getByText('Continue'))
    await screen.findByText('Sign in with Passkey')
    fireEvent.click(screen.getByText('Sign in with Passkey'))

    await waitFor(() => {
      expect(mockCompleteStoredHandoff).toHaveBeenCalled()
    })
    expect(mockNavigate).not.toHaveBeenCalledWith({ path: '/u/5' })
  })

  it('prefills the username from the hash query', () => {
    window.location.hash = '#/auth/passkey?username=SpaceWave'

    render(<PasskeyPage />)

    expect(screen.getByDisplayValue('spacewave')).toBeDefined()
  })

  it('shows fallback guidance after create-account hits username_taken', async () => {
    mockPasskeyCheckUsername.mockResolvedValue({ ok: true })
    mockPasskeyRegisterChallenge.mockResolvedValue({ optionsJson: '{}' })
    mockStartRegistration.mockResolvedValue({
      id: 'cred-4',
      clientExtensionResults: {},
    })
    mockPasskeyConfirmSignup.mockRejectedValue(
      new Error('409 username_taken: Username already exists'),
    )

    render(<PasskeyPage />)

    fireEvent.change(screen.getByPlaceholderText('your-username'), {
      target: { value: 'existing-user' },
    })
    fireEvent.click(screen.getByText('Continue'))
    await screen.findByText('Create New Passkey Account')
    fireEvent.click(screen.getByText('Create New Passkey Account'))

    await screen.findByText(
      'That username is already taken. If this is your account and it does not have a passkey yet, sign in with another method and add one from account settings.',
    )
    expect(screen.getByText('Sign in with Passkey')).toBeDefined()
  })
})
