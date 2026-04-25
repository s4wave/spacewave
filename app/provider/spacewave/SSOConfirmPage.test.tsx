import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SSOConfirmPage } from './SSOConfirmPage.js'

type PendingSSOState = {
  provider: string
  email: string
  nonce: string
  isDesktop: boolean
} | null

const mockNavigate = vi.hoisted(() => vi.fn())
const mockConfirmSSO = vi.hoisted(() => vi.fn())
const mockLoginWithEntityKey = vi.hoisted(() => vi.fn())
const mockLookupProvider = vi.hoisted(() => vi.fn())
const mockGetPendingSSOState = vi.hoisted(() => vi.fn())
const mockClearPendingSSOState = vi.hoisted(() => vi.fn())
const mockUseParams = vi.hoisted(() => vi.fn(() => ({ provider: 'google' })))
const mockWrapPemWithPin = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => mockUseParams(),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => ({
    lookupProvider: mockLookupProvider,
  }),
}))

vi.mock('./useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => null,
}))

vi.mock('./sso-state.js', () => ({
  getPendingSSOState: (): PendingSSOState =>
    mockGetPendingSSOState() as PendingSSOState,
  clearPendingSSOState: (): void => {
    mockClearPendingSSOState()
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
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    confirmSSO = mockConfirmSSO

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

vi.mock('@s4wave/app/prerender/StaticContext.js', () => ({
  useStaticHref: (path: string) => `#${path}`,
}))

function makeProviderRef() {
  return {
    resourceRef: {},
    [Symbol.dispose]: () => {},
  }
}

function submitForm() {
  const form = screen.getByPlaceholderText('your-name').closest('form')
  if (!form) {
    throw new Error('expected SSO confirm form')
  }
  fireEvent.submit(form)
}

async function confirmModal() {
  const confirmButton = await screen.findByRole('button', {
    name: 'Confirm and create account',
  })
  fireEvent.click(confirmButton)
}

describe('SSOConfirmPage', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockConfirmSSO.mockReset()
    mockConfirmSSO.mockResolvedValue({
      accountId: 'acct-1',
      sessionPeerId: 'session-peer-id',
    })
    mockLoginWithEntityKey.mockReset()
    mockLoginWithEntityKey.mockResolvedValue({
      sessionListEntry: { sessionIndex: 7 },
    })
    mockLookupProvider.mockReset()
    mockLookupProvider.mockResolvedValue(makeProviderRef())
    mockGetPendingSSOState.mockReset()
    mockGetPendingSSOState.mockReturnValue({
      provider: 'google',
      email: 'user@example.com',
      nonce: 'nonce-123',
      isDesktop: true,
    })
    mockClearPendingSSOState.mockReset()
    mockUseParams.mockReset()
    mockUseParams.mockReturnValue({ provider: 'google' })
    mockWrapPemWithPin.mockReset()
    mockWrapPemWithPin.mockResolvedValue('pin-wrapped-base64')
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the Optional PIN lock control on the form step', () => {
    render(<SSOConfirmPage />)

    expect(screen.getByText('Optional PIN lock')).toBeDefined()
    expect(screen.getByPlaceholderText('Leave blank to skip')).toBeDefined()
    expect(screen.getByPlaceholderText('Confirm PIN')).toBeDefined()
  })

  it('opens the confirm modal before creating the account and proceeds with pinWrapped false when PIN is blank', async () => {
    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'new-user' },
    })
    submitForm()

    expect(mockConfirmSSO).not.toHaveBeenCalled()
    expect(await screen.findByText('Confirm your username')).toBeDefined()
    expect(screen.getByText('Terms of Service')).toBeDefined()
    expect(screen.getByText('Privacy Policy')).toBeDefined()

    await confirmModal()

    await waitFor(() => {
      expect(mockConfirmSSO).toHaveBeenCalledWith({
        nonce: 'nonce-123',
        username: 'new-user',
        wrappedEntityKey: 'entity-base64',
        entityPeerId: 'entity-peer-id',
        sessionPeerId: 'session-peer-id',
        pinWrapped: false,
      })
    })
    expect(mockWrapPemWithPin).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalled()
    })
    expect(mockClearPendingSSOState).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7' })
  })

  it('blocks the confirm modal and shows a PIN error when only one PIN field is set', () => {
    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'new-user' },
    })
    fireEvent.change(screen.getByPlaceholderText('Leave blank to skip'), {
      target: { value: '1234' },
    })
    submitForm()

    expect(screen.queryByText('Confirm your username')).toBeNull()
    expect(
      screen.getByText('Enter and confirm a PIN, or leave both fields blank'),
    ).toBeDefined()
  })

  it('blocks the confirm modal and shows a PIN error when PIN and confirm PIN do not match', () => {
    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'new-user' },
    })
    fireEvent.change(screen.getByPlaceholderText('Leave blank to skip'), {
      target: { value: '1234' },
    })
    fireEvent.change(screen.getByPlaceholderText('Confirm PIN'), {
      target: { value: 'wrong' },
    })
    submitForm()

    expect(screen.queryByText('Confirm your username')).toBeNull()
    expect(screen.getByText('PINs do not match')).toBeDefined()
  })

  it('wraps the entity PEM and sets pinWrapped true when PIN matches', async () => {
    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'new-user' },
    })
    fireEvent.change(screen.getByPlaceholderText('Leave blank to skip'), {
      target: { value: '1234' },
    })
    fireEvent.change(screen.getByPlaceholderText('Confirm PIN'), {
      target: { value: '1234' },
    })
    submitForm()

    expect(await screen.findByText('Confirm your username')).toBeDefined()
    await confirmModal()

    await waitFor(() => {
      expect(mockWrapPemWithPin).toHaveBeenCalledWith('entity-pem', '1234')
    })
    await waitFor(() => {
      expect(mockConfirmSSO).toHaveBeenCalledWith({
        nonce: 'nonce-123',
        username: 'new-user',
        wrappedEntityKey: 'pin-wrapped-base64',
        entityPeerId: 'entity-peer-id',
        sessionPeerId: 'session-peer-id',
        pinWrapped: true,
      })
    })
    await waitFor(() => {
      expect(mockLoginWithEntityKey).toHaveBeenCalled()
    })
  })

  it('cancels the confirm modal and returns to the username form without creating', async () => {
    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'typoed-name' },
    })
    submitForm()

    expect(await screen.findByText('Confirm your username')).toBeDefined()
    fireEvent.click(
      screen.getByRole('button', { name: /Back to edit username/ }),
    )

    await waitFor(() => {
      expect(screen.queryByText('Confirm your username')).toBeNull()
    })
    expect(mockConfirmSSO).not.toHaveBeenCalled()
    expect(screen.getByPlaceholderText('your-name')).toHaveProperty(
      'value',
      'typoed-name',
    )
  })

  it('keeps the form open on username_taken and shows the username error', async () => {
    mockConfirmSSO.mockRejectedValueOnce(
      new Error('409 username_taken: Username already exists'),
    )

    render(<SSOConfirmPage />)

    fireEvent.change(screen.getByPlaceholderText('your-name'), {
      target: { value: 'taken-name' },
    })
    submitForm()
    await confirmModal()

    expect(await screen.findByText('Username is already taken')).toBeDefined()
    expect(mockLoginWithEntityKey).not.toHaveBeenCalled()
    expect(mockNavigate).not.toHaveBeenCalledWith({ path: '/u/7' })
  })

  it('offers desktop restart when the pending state is missing', () => {
    mockGetPendingSSOState.mockReturnValue(null)

    render(<SSOConfirmPage />)

    fireEvent.click(screen.getByText('Restart sign-in'))

    expect(mockClearPendingSSOState).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/sso/google' })
  })
})
