import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { HandoffPage } from './HandoffPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockStoreHandoff = vi.hoisted(() => vi.fn())
const mockSetSSOStartIntent = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ payload: 'payload-123' }),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => null,
}))

vi.mock('./handoff-state.js', () => ({
  decodeHandoffRequest: () => ({
    clientType: 'cli',
    deviceName: 'Terminal',
    devicePublicKey: new Uint8Array(),
    sessionNonce: 'nonce-1',
  }),
  setStoredHandoffPayload: mockStoreHandoff,
  clearStoredHandoffPayload: vi.fn(),
}))

vi.mock('@s4wave/app/provider/spacewave/sso-start-intent.js', () => ({
  setSSOStartIntent: mockSetSSOStartIntent,
}))

vi.mock('@s4wave/web/ui/login-form.js', () => ({
  LoginForm: ({
    initialUsername,
    onContinueWithPasskey,
    onSignInWithSSO,
    onLoginWithPem,
  }: {
    initialUsername?: string
    onContinueWithPasskey?: () => void
    onSignInWithSSO?: (provider: 'google' | 'github') => void
    onLoginWithPem?: (pem: Uint8Array) => Promise<unknown>
  }) => (
    <div data-testid="login-form" data-initial-username={initialUsername ?? ''}>
      <button onClick={onContinueWithPasskey}>Passkey</button>
      <button onClick={() => onSignInWithSSO?.('google')}>Google</button>
      <span>{onLoginWithPem ? 'PEM available' : 'PEM unavailable'}</span>
    </div>
  ),
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

describe('HandoffPage', () => {
  beforeEach(() => {
    cleanup()
    window.location.hash =
      '#/auth/link/payload-123?intent=signup&username=Spacewave'
    mockNavigate.mockReset()
    mockStoreHandoff.mockReset()
    mockSetSSOStartIntent.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('prefills signup username from the handoff hash query', () => {
    render(<HandoffPage />)

    expect(screen.getByText('Creating a Spacewave CLI account')).toBeDefined()
    expect(screen.getByText('spacewave')).toBeDefined()
    expect(
      screen.getByTestId('login-form').getAttribute('data-initial-username'),
    ).toBe('spacewave')
  })

  it('keeps passkey and SSO on the CLI linking journey', () => {
    render(<HandoffPage />)
    expect(screen.getByText('PEM available')).toBeDefined()

    fireEvent.click(screen.getByText('Passkey'))
    expect(mockStoreHandoff).toHaveBeenCalledWith('payload-123')
    expect(mockNavigate).toHaveBeenCalledWith({
      path: '/auth/passkey?username=spacewave',
    })

    fireEvent.click(screen.getByText('Google'))
    expect(mockSetSSOStartIntent).toHaveBeenCalledWith(
      'google',
      '/auth/link/payload-123',
    )
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/sso/google' })
  })
})
