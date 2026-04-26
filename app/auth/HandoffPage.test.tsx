import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { HandoffPage } from './HandoffPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())

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
  setStoredHandoffPayload: vi.fn(),
}))

vi.mock('@s4wave/web/ui/login-form.js', () => ({
  LoginForm: ({ initialUsername }: { initialUsername?: string }) => (
    <div data-testid="login-form" data-initial-username={initialUsername ?? ''}>
      login-form
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

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
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
})
