import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import {
  SpaceRootRuntimeStatus,
  SpaceRootStatus,
  type SpaceRootAliasRecord,
  type WatchSpaceRootRuntimeResponse,
} from '@s4wave/sdk/root/root.pb.js'
import { SessionSelector } from './SessionSelector.js'

const mockUseSessionList = vi.hoisted(() => vi.fn())
const mockUseSessionMetadata = vi.hoisted(() => vi.fn())
const mockUseSessionAccountStatuses = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseSpaceRootAliases = vi.hoisted(() =>
  vi.fn(() => ({
    loading: false,
    value: { records: [] as SpaceRootAliasRecord[] },
  })),
)
const mockUseSpaceRootRuntime = vi.hoisted(() =>
  vi.fn(
    (
      _aliasId: string | null,
    ): { loading: boolean; value: WatchSpaceRootRuntimeResponse | null } => ({
      loading: false,
      value: null,
    }),
  ),
)

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: mockUseSessionList,
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('@s4wave/app/hooks/useSessionAccountStatuses.js', () => ({
  useSessionAccountStatuses: mockUseSessionAccountStatuses,
}))

vi.mock('@s4wave/app/hooks/useSpaceRootAliases.js', () => ({
  useSpaceRootAliases: mockUseSpaceRootAliases,
}))

vi.mock('@s4wave/app/hooks/useSpaceRootRuntime.js', () => ({
  useSpaceRootRuntime: mockUseSpaceRootRuntime,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))
vi.mock('@s4wave/web/router/app-path.js', async (importOriginal) => ({
  ...(await importOriginal()),
  setAppPath: (path: string) => {
    mockNavigate({ path })
  },
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/ui/button.js', () => ({
  Button: (props: { children?: ReactNode; onClick?: () => void }) => (
    <button type="button" onClick={props.onClick}>
      {props.children}
    </button>
  ),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('SessionSelector', () => {
  it('opens the first ready state root when there are no local sessions', () => {
    mockUseSessionList.mockReturnValue({
      loading: false,
      error: null,
      retry: vi.fn(),
      value: { sessions: [] },
    })
    mockUseSessionAccountStatuses.mockReturnValue(new Map())
    mockUseSpaceRootAliases.mockReturnValue({
      loading: false,
      value: {
        records: [
          {
            aliasId: 'spacewave',
            displayName: '.spacewave',
            status: SpaceRootStatus.SpaceRootStatus_READY,
            native: { path: '/home/user/.spacewave' },
          },
        ],
      },
    })

    mockUseSpaceRootRuntime.mockReturnValue({
      loading: false,
      value: {
        status: SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_READY,
        runtimeSessions: [
          {
            session: { sessionIndex: 1 },
            metadata: { providerId: 'local', providerAccountId: 'acct-1' },
            spaces: [{ spaceMeta: { name: 'aperture' } }],
          },
        ],
      } as WatchSpaceRootRuntimeResponse,
    })

    render(<SessionSelector />)

    expect(screen.getByText('.spacewave')).toBeTruthy()
    expect(mockUseSpaceRootRuntime).toHaveBeenLastCalledWith('spacewave')
    expect(screen.getByText('Local account')).toBeTruthy()
    expect(screen.getByText('aperture')).toBeTruthy()

    // Closing the default root keeps it closed.
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    expect(mockUseSpaceRootRuntime).toHaveBeenLastCalledWith(null)
  })

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

  it('selects an account through the shared session route', () => {
    mockUseSessionList.mockReturnValue({
      loading: false,
      error: null,
      retry: vi.fn(),
      value: {
        sessions: [{ sessionIndex: 12 }],
      },
    })
    mockUseSessionMetadata.mockReturnValue({
      providerId: 'local',
      displayName: 'Work',
    })
    mockUseSessionAccountStatuses.mockReturnValue(new Map())

    render(<SessionSelector />)
    fireEvent.click(screen.getByText('Work'))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/12/' })
  })
})
