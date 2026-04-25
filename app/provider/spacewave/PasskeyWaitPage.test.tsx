import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PasskeyWaitPage } from './PasskeyWaitPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())
const mockStartDesktopPasskey = vi.hoisted(() => vi.fn())
const mockLoginWithEntityKey = vi.hoisted(() => vi.fn())
const mockBase64ToBytes = vi.hoisted(() => vi.fn())
const mockUnwrapPemWithPin = vi.hoisted(() => vi.fn())
const mockIsPasskeyPrfPinWrapped = vi.hoisted(() => vi.fn())
const mockUnwrapPemWithPasskeyPrf = vi.hoisted(() => vi.fn())
const mockSetPendingDesktopPasskeyState = vi.hoisted(() => vi.fn())
const mockRoot = vi.hoisted(() => ({
  lookupProvider: mockLookupProvider,
}))

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockRoot,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    startDesktopPasskey = mockStartDesktopPasskey
    loginWithEntityKey = mockLoginWithEntityKey
    unwrapPemWithPin = (wrappedPemBase64: string, pin: string) =>
      mockUnwrapPemWithPin(wrappedPemBase64, pin) as Promise<{
        pemPrivateKey: Uint8Array
      }>
  },
}))

vi.mock('./keypair-utils.js', () => ({
  base64ToBytes: (...args: [string]): Uint8Array =>
    mockBase64ToBytes(...args) as Uint8Array,
  unwrapPemWithPin: async (
    ...args: [unknown, string, string]
  ): Promise<Uint8Array> => {
    const resp = (await mockUnwrapPemWithPin(args[1], args[2])) as
      | Uint8Array
      | { pemPrivateKey?: Uint8Array }
    return resp instanceof Uint8Array ? resp : (
        (resp.pemPrivateKey ?? new Uint8Array())
      )
  },
}))

vi.mock('./passkey-prf.js', () => ({
  isPasskeyPrfPinWrapped: (...args: [string]): boolean =>
    mockIsPasskeyPrfPinWrapped(...args) as boolean,
  unwrapPemWithPasskeyPrf: (
    ...args: [unknown, string, string, Uint8Array]
  ): Promise<Uint8Array> =>
    mockUnwrapPemWithPasskeyPrf(...args) as Promise<Uint8Array>,
}))

vi.mock('./desktop-passkey-state.js', () => ({
  setPendingDesktopPasskeyState: (state: unknown): void => {
    mockSetPendingDesktopPasskeyState(state)
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

describe('PasskeyWaitPage', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockStartDesktopPasskey.mockReset()
    mockLoginWithEntityKey.mockReset()
    mockBase64ToBytes.mockReset()
    mockUnwrapPemWithPin.mockReset()
    mockIsPasskeyPrfPinWrapped.mockReset()
    mockUnwrapPemWithPasskeyPrf.mockReset()
    mockSetPendingDesktopPasskeyState.mockReset()
    mockBase64ToBytes.mockReturnValue(new Uint8Array([1, 2, 3]))
    mockIsPasskeyPrfPinWrapped.mockReturnValue(false)
    mockLoginWithEntityKey.mockResolvedValue({
      sessionListEntry: { sessionIndex: 9 },
    })
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('logs in immediately for a linked desktop passkey result', async () => {
    mockStartDesktopPasskey.mockResolvedValue({
      result: {
        case: 'linked',
        value: {
          encryptedBlob: 'ZW50aXR5',
          prfCapable: false,
          pinWrapped: false,
        },
      },
    })

    render(<PasskeyWaitPage />)

    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalled()
    })
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/9' })
  })

  it('prompts for a PIN and unwraps before login when the linked key is pin-wrapped', async () => {
    mockStartDesktopPasskey.mockResolvedValue({
      result: {
        case: 'linked',
        value: {
          encryptedBlob: 'wrapped-blob',
          prfCapable: false,
          pinWrapped: true,
        },
      },
    })
    mockUnwrapPemWithPin.mockResolvedValue({
      pemPrivateKey: new Uint8Array([4, 5, 6]),
    })

    render(<PasskeyWaitPage />)

    await screen.findByText('Enter your PIN')
    fireEvent.change(screen.getByPlaceholderText('PIN'), {
      target: { value: '1234' },
    })
    fireEvent.keyDown(screen.getByPlaceholderText('PIN'), { key: 'Enter' })

    await waitFor(() => {
      expect(mockUnwrapPemWithPin).toHaveBeenCalledWith('wrapped-blob', '1234')
    })
    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalledWith(
        new Uint8Array([4, 5, 6]),
      )
    })
  })

  it('stores new-account desktop passkey state and navigates to confirm', async () => {
    mockStartDesktopPasskey.mockResolvedValue({
      result: {
        case: 'newAccount',
        value: {
          nonce: 'nonce-123',
          username: 'new-user',
          credentialJson: '{"id":"cred-1"}',
          prfCapable: true,
          prfSalt: 'salt-1',
          prfOutput: 'output-1',
        },
      },
    })

    render(<PasskeyWaitPage />)

    await waitFor(() => {
      expect(mockSetPendingDesktopPasskeyState).toHaveBeenCalledWith({
        nonce: 'nonce-123',
        username: 'new-user',
        credentialJson: '{"id":"cred-1"}',
        prfCapable: true,
        prfSalt: 'salt-1',
        prfOutput: 'output-1',
      })
    })
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/passkey/confirm' })
  })

  it('offers retry from the desktop passkey error state', async () => {
    mockStartDesktopPasskey
      .mockRejectedValueOnce(new Error('desktop failed'))
      .mockResolvedValueOnce({
        result: {
          case: 'linked',
          value: {
            encryptedBlob: 'ZW50aXR5',
            prfCapable: false,
            pinWrapped: false,
          },
        },
      })

    render(<PasskeyWaitPage />)

    await screen.findByText('Passkey sign-in failed')
    fireEvent.click(screen.getByText('Open again'))
    await waitFor(() => {
      expect(mockStartDesktopPasskey).toHaveBeenCalledTimes(2)
    })
  })

  it('lets the user cancel from the desktop passkey error state', async () => {
    mockStartDesktopPasskey.mockRejectedValueOnce(new Error('desktop failed'))

    render(<PasskeyWaitPage />)

    await screen.findByText('Passkey sign-in failed')
    fireEvent.click(screen.getByText('Back to login'))
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/login' })
  })
})
