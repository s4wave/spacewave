import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { SessionSelector } from './SessionSelector.js'

const mockUseSessionList = vi.hoisted(() => vi.fn())
const mockUseSessionMetadata = vi.hoisted(() => vi.fn())
const mockUseSessionAccountStatuses = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: mockUseSessionList,
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('@s4wave/app/hooks/useSessionAccountStatuses.js', () => ({
  useSessionAccountStatuses: mockUseSessionAccountStatuses,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: () => null,
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/ui/button.js', () => ({
  Button: (props: { children?: ReactNode; onClick?: () => void }) => (
    <button onClick={props.onClick}>{props.children}</button>
  ),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('SessionSelector', () => {
  it('renders an inactive pill for dormant cloud sessions', () => {
    mockUseSessionList.mockReturnValue({
      loading: false,
      error: null,
      retry: vi.fn(),
      value: {
        sessions: [{ sessionIndex: 7 }],
      },
    })
    mockUseSessionMetadata.mockReturnValue({
      providerId: 'spacewave',
      providerDisplayName: 'Cloud',
      displayName: 'Dormant Cloud',
    })
    mockUseSessionAccountStatuses.mockReturnValue(
      new Map([[7, ProviderAccountStatus.ProviderAccountStatus_DORMANT]]),
    )

    render(<SessionSelector />)

    expect(screen.getByText('Dormant Cloud')).toBeTruthy()
    expect(screen.getAllByText('Cloud')).toHaveLength(2)
    expect(screen.getByText('(Inactive)')).toBeTruthy()
  })

  it('omits the inactive pill for ready cloud sessions', () => {
    mockUseSessionList.mockReturnValue({
      loading: false,
      error: null,
      retry: vi.fn(),
      value: {
        sessions: [{ sessionIndex: 8 }],
      },
    })
    mockUseSessionMetadata.mockReturnValue({
      providerId: 'spacewave',
      providerDisplayName: 'Cloud',
      displayName: 'Active Cloud',
    })
    mockUseSessionAccountStatuses.mockReturnValue(
      new Map([[8, ProviderAccountStatus.ProviderAccountStatus_READY]]),
    )

    render(<SessionSelector />)

    expect(screen.getByText('Active Cloud')).toBeTruthy()
    expect(screen.queryByText('(Inactive)')).toBeNull()
  })

  it('de-emphasizes linked local sessions with a linked label', () => {
    mockUseSessionList.mockReturnValue({
      loading: false,
      error: null,
      retry: vi.fn(),
      value: {
        sessions: [{ sessionIndex: 9 }],
      },
    })
    mockUseSessionMetadata.mockReturnValue({
      providerId: 'local',
      providerDisplayName: 'Local',
      displayName: 'Linked Local',
      cloudAccountId: 'cloud-acct-1',
    })
    mockUseSessionAccountStatuses.mockReturnValue(new Map())

    render(<SessionSelector />)

    expect(screen.getByText('Linked Local')).toBeTruthy()
    expect(screen.getByText('(linked)')).toBeTruthy()
  })
})
