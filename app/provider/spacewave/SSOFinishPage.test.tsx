import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SSOFinishPage } from './SSOFinishPage.js'
import {
  clearSSOBrowserBinding,
  getSSOBrowserBinding,
  setSSOBrowserBinding,
} from './sso-start-intent.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockRoot = vi.hoisted(() => ({ root: true }))
const mockExchange = vi.hoisted(() => vi.fn())
const mockLogin = vi.hoisted(() => vi.fn())
const mockProvider = vi.hoisted(() => ({
  ssoNonceExchange: mockExchange,
  loginWithEntityKey: mockLogin,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ nonce: 'finish-nonce' }),
}))
vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))
vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockRoot,
}))
vi.mock('./auth-flow-shared.js', () => ({
  getErrorMessage: (err: unknown, fallback: string) =>
    err instanceof Error ? err.message : fallback,
  withSpacewaveProvider: async <T,>(
    _root: unknown,
    fn: (provider: typeof mockProvider) => Promise<T>,
  ) => await fn(mockProvider),
}))
vi.mock('./keypair-utils.js', () => ({
  base64ToBytes: (value: string) => new TextEncoder().encode(value),
  unwrapPemWithPin: vi.fn(),
}))
vi.mock('@s4wave/app/auth/AuthScreenLayout.js', () => ({
  AuthScreenLayout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('@s4wave/web/ui/loading/Spinner.js', () => ({
  Spinner: () => <div>spinner</div>,
}))
vi.mock('./SSOUnlockCard.js', () => ({
  SSOUnlockCard: () => <div>unlock</div>,
}))
vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div>logo</div>,
}))

const binding = {
  verifier: 'verifier',
  verifierHash: 'commitment',
  devicePublicKey: 'public-key',
  devicePrivateKey: 'private-key',
}

describe('SSOFinishPage tab binding', () => {
  beforeEach(() => {
    cleanup()
    sessionStorage.clear()
    clearSSOBrowserBinding()
    mockNavigate.mockReset()
    mockExchange.mockReset()
    mockLogin.mockReset()
  })

  afterEach(cleanup)

  it('does not exchange or mount a finish URL in an independent context without the binding', async () => {
    render(<SSOFinishPage />)

    await screen.findByText(
      /This sign-in link requires the browser state created when sign-in started/,
    )
    expect(mockExchange).not.toHaveBeenCalled()
    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('retains the binding through exchange failure and restarts instead of replaying', async () => {
    setSSOBrowserBinding(binding)
    mockExchange.mockRejectedValue(new Error('Temporary network failure'))

    render(<SSOFinishPage />)

    await screen.findByText(/Temporary network failure/)
    expect(getSSOBrowserBinding()).toEqual(binding)
    expect(mockExchange).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByText('Start sign-in again'))

    expect(getSSOBrowserBinding()).toBeNull()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/login' })
    expect(mockExchange).toHaveBeenCalledTimes(1)
  })

  it('retries with the retained binding and clears it only after mounting succeeds', async () => {
    setSSOBrowserBinding(binding)
    mockExchange.mockResolvedValue({
      linked: true,
      encryptedBlob: 'entity-pem',
      pinWrapped: false,
    })
    mockLogin.mockResolvedValue({ sessionListEntry: { sessionIndex: 9 } })

    render(<SSOFinishPage />)

    await waitFor(() => {
      expect(mockExchange).toHaveBeenCalledWith(
        {
          nonce: 'finish-nonce',
          verifier: new TextEncoder().encode('verifier'),
          devicePrivateKey: new TextEncoder().encode('private-key'),
        },
        expect.any(AbortSignal),
      )
    })
    await waitFor(() => expect(mockLogin).toHaveBeenCalled())
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/9' })
    expect(getSSOBrowserBinding()).toBeNull()
  })
})
