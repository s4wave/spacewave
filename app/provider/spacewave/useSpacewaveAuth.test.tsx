import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSpacewaveAuth } from './useSpacewaveAuth.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockNavigateToSession = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: vi.fn((resource) => {
    if (resource === 'root-resource') {
      return 'provider-resource'
    }
    if (resource === 'provider-resource') {
      return 'cloud-config-resource'
    }
    return null
  }),
  useResourceValue: vi.fn((resource) => {
    if (resource === 'root-resource') {
      return { lookupProvider: vi.fn() }
    }
    if (resource === 'cloud-config-resource') {
      return {
        ssoBaseUrl: 'https://account.spacewave.test/auth/sso',
        googleSsoEnabled: true,
        githubSsoEnabled: true,
      }
    }
    return null
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  RootContext: {
    useContext: () => 'root-resource',
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class {
    constructor(_resourceRef: unknown) {}

    getCloudProviderConfig = vi.fn()
  },
}))

function HookHarness() {
  const auth = useSpacewaveAuth(mockNavigateToSession)
  return (
    <>
      <button onClick={() => void auth.handleContinueWithPasskey()}>
        passkey
      </button>
      <button onClick={() => void auth.handleSignInWithSSO('google')}>
        google-sso
      </button>
      <button onClick={() => void auth.handleSignInWithSSO('github')}>
        github-sso
      </button>
    </>
  )
}

describe('useSpacewaveAuth SSO', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockNavigateToSession.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('navigates to the SSO wait page for Google', () => {
    render(<HookHarness />)
    fireEvent.click(screen.getByText('google-sso'))
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/sso/google' })
    expect(mockNavigateToSession).not.toHaveBeenCalled()
  })

  it('navigates to the SSO wait page for GitHub', () => {
    render(<HookHarness />)
    fireEvent.click(screen.getByText('github-sso'))
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/sso/github' })
    expect(mockNavigateToSession).not.toHaveBeenCalled()
  })

  it('navigates to the desktop passkey wait page', () => {
    render(<HookHarness />)
    fireEvent.click(screen.getByText('passkey'))
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/auth/passkey/wait' })
    expect(mockNavigateToSession).not.toHaveBeenCalled()
  })
})
