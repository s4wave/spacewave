import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: vi.fn(() => mockNavigate),
  useParentPaths: vi.fn(() => ['/setup']),
  usePath: vi.fn(() => '/setup/link-device'),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: vi.fn(() => ({ value: null })) },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: vi.fn(() => null),
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: vi.fn(() => ({
    data: null,
    loading: false,
    error: null,
  })),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useOptionalLocalSessionOnboardingContext: vi.fn(() => ({
    markProviderChoiceComplete: vi.fn(),
  })),
}))

vi.mock('./SetupPageLayout.js', () => ({
  SetupPageLayout: ({
    children,
    title,
  }: {
    children: React.ReactNode
    title: string
  }) => (
    <div>
      <h1>{title}</h1>
      {children}
    </div>
  ),
}))

import { LinkDeviceWizard } from './LinkDeviceWizard.js'

describe('LinkDeviceWizard', () => {
  beforeEach(() => {
    mockNavigate.mockClear()
    mockUseSessionInfo.mockReset()
    mockUseSessionInfo.mockReturnValue({
      error: null,
      loading: false,
      providerId: 'local',
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('renders cloud relay pairing options for spacewave providers', () => {
    mockUseSessionInfo.mockReturnValue({
      error: null,
      loading: false,
      providerId: 'spacewave',
    })

    render(<LinkDeviceWizard />)

    expect(screen.getByText('Generate code for another device')).toBeDefined()
    expect(screen.getByText('Enter a code from another device')).toBeDefined()
    expect(screen.queryByText('Direct connection (show QR)')).toBeNull()
    expect(screen.queryByText('Direct connection (scan QR)')).toBeNull()
  })
})
