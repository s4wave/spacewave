import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PasskeyConfirmPage } from './PasskeyConfirmPage.js'

type PendingPasskeyState = {
  nonce: string
  username: string
  credentialJson: string
  prfCapable: boolean
  prfSalt: string
  prfOutput: string
} | null

const mockNavigate = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())
const mockGetPendingDesktopPasskeyState = vi.hoisted(() => vi.fn())
const mockClearPendingDesktopPasskeyState = vi.hoisted(() => vi.fn())
const mockConfirmDesktopPasskey = vi.hoisted(() => vi.fn())
const mockLoginWithEntityKey = vi.hoisted(() => vi.fn())
const mockWrapPemWithPin = vi.hoisted(() => vi.fn())

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

vi.mock('./desktop-passkey-state.js', () => ({
  getPendingDesktopPasskeyState: (): PendingPasskeyState =>
    mockGetPendingDesktopPasskeyState() as PendingPasskeyState,
  clearPendingDesktopPasskeyState: () => {
    mockClearPendingDesktopPasskeyState()
  },
}))

vi.mock('./keypair-utils.js', () => ({
  dnsLabelRegex: /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/,
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
  wrapPemWithPin: (...args: [unknown, string, string]): Promise<string> =>
    mockWrapPemWithPin(args[1], args[2]) as Promise<string>,
  base64ToBytes: () => new Uint8Array([1, 2, 3]),
}))

vi.mock('./passkey-prf.js', () => ({
  wrapPemWithPasskeyPrf: vi.fn(),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    confirmDesktopPasskey = mockConfirmDesktopPasskey
    loginWithEntityKey = mockLoginWithEntityKey
  },
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

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div>logo</div>,
}))

function makeProviderRef() {
  return {
    resourceRef: {},
    [Symbol.dispose]: () => {},
  }
}

describe('PasskeyConfirmPage', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockGetPendingDesktopPasskeyState.mockReset()
    mockGetPendingDesktopPasskeyState.mockReturnValue({
      nonce: 'nonce-123',
      username: 'pending-user',
      credentialJson: '{"id":"cred-1"}',
      prfCapable: false,
      prfSalt: '',
      prfOutput: '',
    })
    mockClearPendingDesktopPasskeyState.mockReset()
    mockConfirmDesktopPasskey.mockReset()
    mockConfirmDesktopPasskey.mockResolvedValue({
      accountId: 'acct-1',
      sessionPeerId: 'session-peer-id',
    })
    mockLoginWithEntityKey.mockReset()
    mockLoginWithEntityKey.mockResolvedValue({
      sessionListEntry: { sessionIndex: 11 },
    })
    mockWrapPemWithPin.mockReset()
    mockWrapPemWithPin.mockResolvedValue('pin-wrapped-base64')
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('keeps the form open on username_taken and shows the username error', async () => {
    mockConfirmDesktopPasskey.mockRejectedValueOnce(
      new Error('409 username_taken: Username already exists'),
    )

    render(<PasskeyConfirmPage />)

    fireEvent.change(screen.getByDisplayValue('pending-user'), {
      target: { value: 'taken-name' },
    })
    fireEvent.click(screen.getByText('Create account'))

    expect(await screen.findByText('Username is already taken')).toBeDefined()
    expect(mockLoginWithEntityKey).not.toHaveBeenCalled()
  })

  it('offers restart when the pending desktop passkey state is missing', () => {
    mockGetPendingDesktopPasskeyState.mockReturnValue(null)

    render(<PasskeyConfirmPage />)

    fireEvent.click(screen.getByText('Restart sign-in'))

    expect(mockClearPendingDesktopPasskeyState).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/passkey/wait' })
  })

  it('confirms and logs in for a valid desktop passkey account creation', async () => {
    render(<PasskeyConfirmPage />)

    fireEvent.change(screen.getByDisplayValue('pending-user'), {
      target: { value: 'new-user' },
    })
    fireEvent.click(screen.getByText('Create account'))

    await waitFor(() => {
      expect(mockConfirmDesktopPasskey).toHaveBeenCalledWith({
        nonce: 'nonce-123',
        username: 'new-user',
        credentialJson: '{"id":"cred-1"}',
        wrappedEntityKey: 'entity-base64',
        entityPeerId: 'entity-peer-id',
        sessionPeerId: 'session-peer-id',
        pinWrapped: false,
        prfCapable: false,
        prfSalt: '',
        authParams: '',
      })
    })
    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalled()
    })
    expect(mockClearPendingDesktopPasskeyState).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/11' })
  })
})
