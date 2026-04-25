import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

const CDN_SPACE_ID = '01kpm6m5mg9ncme4ve3jraxv5n'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockToastError = vi.hoisted(() => vi.fn())
const mockUseWatchStateRpc = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockSessionUseContext = vi.hoisted(() => vi.fn())
const mockOrgListUseContextSafe = vi.hoisted(() => vi.fn())
const mockOnboardingUseContextSafe = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUseMountAccount = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseSessionIndex = vi.hoisted(() => vi.fn())
const mockUseSessionNavigate = vi.hoisted(() => vi.fn())
const renderedDashboard = vi.hoisted(
  () =>
    vi.fn() as unknown as ReturnType<
      typeof vi.fn<(props: unknown) => React.ReactElement>
    >,
)

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: mockUseWatchStateRpc,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: mockToastError },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: mockUseResource,
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: mockSessionUseContext },
  useSessionIndex: () => mockUseSessionIndex(),
  useSessionNavigate: () => mockUseSessionNavigate,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: { useContextSafe: mockOrgListUseContextSafe },
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: mockOnboardingUseContextSafe,
  },
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: mockUseMountAccount,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('./SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

vi.mock('./dashboard/SessionDashboard.js', () => ({
  SessionDashboard: (props: { onQuickstartClick?: (id: string) => void }) => {
    renderedDashboard(props)
    return (
      <button
        data-testid="quickstart-drive"
        onClick={() => props.onQuickstartClick?.('drive')}
      >
        Drive
      </button>
    )
  },
}))

vi.mock('@s4wave/app/quickstart/create.js', () => ({
  __esModule: true,
}))

import { SessionDashboardContainer } from './SessionDashboardContainer.js'

function getRenderedSpaces() {
  const props = renderedDashboard.mock.calls[0]?.[0] as
    | {
        spaces?: Array<{ id: string; name: string; orgId?: string }>
      }
    | undefined
  expect(props).toBeDefined()
  return props?.spaces ?? []
}

describe('SessionDashboardContainer', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockToastError.mockReset()
    renderedDashboard.mockReset()

    mockUseWatchStateRpc.mockReturnValue({ spacesList: [] })
    mockUseResource.mockReturnValue({
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    mockUseResourceValue.mockReturnValue({
      createSpace: vi.fn(),
      deleteSpace: vi.fn(),
    })
    mockSessionUseContext.mockReturnValue({ value: {} })
    mockOrgListUseContextSafe.mockReturnValue({ organizations: [] })
    mockOnboardingUseContextSafe.mockReturnValue({
      onboarding: null,
      isPendingDelete: false,
      isReadOnlyGrace: false,
    })
    mockUseSessionInfo.mockReturnValue({
      providerId: 'local',
      accountId: 'acct',
      isCloud: false,
    })
    mockUseMountAccount.mockReturnValue({ value: null })
    mockUseRootResource.mockReturnValue({ value: null })
    mockUseSessionIndex.mockReturnValue(1)
    mockUseSessionNavigate.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('navigates to ./new/drive on quickstart click without running a pipeline', () => {
    render(<SessionDashboardContainer />)
    const tile = screen.getByTestId('quickstart-drive')
    fireEvent.click(tile)

    expect(mockUseSessionNavigate).toHaveBeenCalledWith({ path: 'new/drive' })
    const props = renderedDashboard.mock.calls[0]?.[0] as
      | { quickstartLoading?: unknown }
      | undefined
    expect(props).toBeDefined()
    expect(props!.quickstartLoading).toBeUndefined()
  })

  it('blocks quickstart navigation for read-only accounts with a toast', () => {
    mockOnboardingUseContextSafe.mockReturnValue({
      onboarding: null,
      isPendingDelete: true,
      isReadOnlyGrace: false,
    })
    render(<SessionDashboardContainer />)
    const tile = screen.getByTestId('quickstart-drive')
    fireEvent.click(tile)

    expect(mockToastError).toHaveBeenCalledWith(
      'This cloud account is read-only',
    )
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(mockUseSessionNavigate).not.toHaveBeenCalled()
  })

  it('injects the CDN space under the owning org group', () => {
    const cdnResource = {
      value: CDN_SPACE_ID,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    mockUseResource.mockReturnValue(cdnResource)
    mockUseResourceValue.mockImplementation((resource) => {
      if (resource === cdnResource) return CDN_SPACE_ID
      return {
        createSpace: vi.fn(),
        deleteSpace: vi.fn(),
      }
    })
    mockOrgListUseContextSafe.mockReturnValue({
      organizations: [
        {
          id: 'org-1',
          displayName: 'Aperture Robotics',
          spaceIds: [CDN_SPACE_ID],
        },
      ],
    })

    render(<SessionDashboardContainer />)

    expect(getRenderedSpaces()).toContainEqual({
      id: CDN_SPACE_ID,
      name: 'Spacewave CDN',
      orgId: 'org-1',
    })
  })

  it('does not inject the CDN space when no org claims it', () => {
    const cdnResource = {
      value: CDN_SPACE_ID,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    mockUseResource.mockReturnValue(cdnResource)
    mockUseResourceValue.mockImplementation((resource) => {
      if (resource === cdnResource) return CDN_SPACE_ID
      return {
        createSpace: vi.fn(),
        deleteSpace: vi.fn(),
      }
    })

    render(<SessionDashboardContainer />)

    expect(getRenderedSpaces()).not.toContainEqual(
      expect.objectContaining({ id: CDN_SPACE_ID }),
    )
  })

  it('does not inject the CDN space when the CDN resource is null', () => {
    const cdnResource = {
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    mockUseResource.mockReturnValue(cdnResource)
    mockUseResourceValue.mockImplementation((resource) => {
      if (resource === cdnResource) return null
      return {
        createSpace: vi.fn(),
        deleteSpace: vi.fn(),
      }
    })
    mockOrgListUseContextSafe.mockReturnValue({
      organizations: [
        {
          id: 'org-1',
          displayName: 'Aperture Robotics',
          spaceIds: [CDN_SPACE_ID],
        },
      ],
    })

    render(<SessionDashboardContainer />)

    expect(getRenderedSpaces()).not.toContainEqual(
      expect.objectContaining({ id: CDN_SPACE_ID }),
    )
  })
})
