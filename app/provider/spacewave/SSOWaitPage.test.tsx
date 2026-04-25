import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SSOWaitPage } from './SSOWaitPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())
const mockStartDesktopSSO = vi.hoisted(() => vi.fn())
const mockLoginWithEntityKey = vi.hoisted(() => vi.fn())
const mockUnwrapPemWithPin = vi.hoisted(() => vi.fn())
const mockBytesToBase64 = vi.hoisted(() => vi.fn())
const mockSetPendingSSOState = vi.hoisted(() => vi.fn())
const mockRoot = vi.hoisted(() => ({
  lookupProvider: mockLookupProvider,
}))

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ provider: 'github' }),
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

    startDesktopSSO = mockStartDesktopSSO
    loginWithEntityKey = mockLoginWithEntityKey
  },
}))

vi.mock('./keypair-utils.js', () => ({
  bytesToBase64: (...args: [Uint8Array]): string =>
    mockBytesToBase64(...args) as string,
  unwrapPemWithPin: async (
    ...args: [unknown, string, string]
  ): Promise<Uint8Array> =>
    (await mockUnwrapPemWithPin(args[1], args[2])) as Uint8Array,
}))

vi.mock('./sso-state.js', () => ({
  setPendingSSOState: (state: unknown): void => {
    mockSetPendingSSOState(state)
  },
}))

vi.mock('./useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => ({
    ssoBaseUrl: 'https://account.test/auth/sso',
  }),
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

describe('SSOWaitPage', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockStartDesktopSSO.mockReset()
    mockLoginWithEntityKey.mockReset()
    mockUnwrapPemWithPin.mockReset()
    mockBytesToBase64.mockReset()
    mockSetPendingSSOState.mockReset()
    mockBytesToBase64.mockReturnValue('wrapped-blob')
    mockLoginWithEntityKey.mockResolvedValue({
      sessionListEntry: { sessionIndex: 7 },
    })
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('logs in immediately for a linked desktop SSO result with plaintext PEM', async () => {
    const pem = new Uint8Array([1, 2, 3])
    mockStartDesktopSSO.mockResolvedValue({
      result: {
        case: 'linked',
        value: {
          pemPrivateKey: pem,
          pinWrapped: false,
        },
      },
    })

    render(<SSOWaitPage />)

    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalledWith(pem)
    })
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7' })
  })

  it('prompts for a PIN and unwraps before login when linked SSO is pin-wrapped', async () => {
    const wrapped = new TextEncoder().encode('wrapped-blob')
    const pem = new Uint8Array([4, 5, 6])
    mockStartDesktopSSO.mockResolvedValue({
      result: {
        case: 'linked',
        value: {
          pemPrivateKey: wrapped,
          pinWrapped: true,
          username: 'cjs',
          email: 'cjs@example.com',
        },
      },
    })
    mockUnwrapPemWithPin.mockResolvedValue(pem)

    render(<SSOWaitPage />)

    await screen.findByText('Enter your PIN to finish signing in')
    expect(screen.getByText('cjs')).toBeTruthy()
    expect(screen.getByText('cjs@example.com')).toBeTruthy()
    fireEvent.change(screen.getByPlaceholderText('Enter your PIN'), {
      target: { value: '1234' },
    })
    fireEvent.keyDown(screen.getByPlaceholderText('Enter your PIN'), {
      key: 'Enter',
    })

    await waitFor(() => {
      expect(mockBytesToBase64).toHaveBeenCalledWith(wrapped)
    })
    await waitFor(() => {
      expect(mockUnwrapPemWithPin).toHaveBeenCalledWith('wrapped-blob', '1234')
    })
    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalledWith(pem)
    })
  })
})
