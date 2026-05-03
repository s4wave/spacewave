import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
} from '@s4wave/core/sobject/sobject.pb.js'

const SPACE_ID = '01kpm6m5mg9ncme4ve3jraxv5n'

const mockUseParams = vi.hoisted(() => vi.fn())
const mockUseParentPaths = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockUseWatchStateRpc = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockSessionUseContext = vi.hoisted(() => vi.fn())
const mockOrgListUseContextSafe = vi.hoisted(() => vi.fn())
const mockRepairSharedObject = vi.hoisted(() => vi.fn())
const mockReinitializeSharedObject = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr-react', () => ({
  DebugInfo: ({ children }: { children?: ReactNode }) => <>{children}</>,
  DebugInfoProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
  useWatchStateRpc: mockUseWatchStateRpc,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
  useParentPaths: mockUseParentPaths,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: mockSessionUseContext },
  SharedObjectContext: {
    Provider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  },
  SharedObjectBodyContext: {
    Provider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  },
  useSessionNavigate: () => mockNavigateSession,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: { useContextSafe: mockOrgListUseContextSafe },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: mockUseResource,
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/app/sobject/SharedObjectBodyContainer.js', () => ({
  SharedObjectBodyContainer: () => <div data-testid="shared-object-body" />,
}))

vi.mock('@s4wave/web/ui/loading/LoadingCard.js', () => ({
  LoadingCard: () => <div data-testid="loading-card" />,
}))

vi.mock('@s4wave/web/ui/ErrorState.js', () => ({
  ErrorState: () => <div data-testid="error-state" />,
}))

vi.mock('@s4wave/app/prerender/StaticContext.js', () => ({
  useStaticHref: () => '/dmca',
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: () => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({ providerId: '', accountId: '' }),
}))

vi.mock('./dashboard/AccountDashboardStateContext.js', () => ({
  AccountDashboardStateProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('./dashboard/AuthConfirmDialog.js', () => ({
  AuthConfirmDialog: () => null,
}))

vi.mock('./SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

import { SessionSharedObjectContainer } from './SessionSharedObjectContainer.js'

function setWatchMocks(resourcesList: unknown, health: unknown) {
  mockUseWatchStateRpc.mockReset()
  let callCount = 0
  mockUseWatchStateRpc.mockImplementation(() => {
    callCount += 1
    return callCount % 2 === 1 ? resourcesList : health
  })
}

function buildSpaceListEntry(source: string) {
  return {
    entry: {
      ref: {
        providerResourceRef: {
          id: SPACE_ID,
        },
      },
      source,
    },
  }
}

function setResourceMocks(sharedObject: unknown, body: unknown) {
  mockUseResource.mockReset()
  let callCount = 0
  mockUseResource.mockImplementation(() => {
    callCount += 1
    return callCount % 2 === 1 ? sharedObject : body
  })
}

describe('SessionSharedObjectContainer', () => {
  beforeEach(() => {
    mockUseParams.mockReset()
    mockUseParentPaths.mockReset()
    mockNavigate.mockReset()
    mockNavigateSession.mockReset()
    mockUseResourceValue.mockReset()
    mockSessionUseContext.mockReset()
    mockOrgListUseContextSafe.mockReset()
    mockRepairSharedObject.mockReset()
    mockReinitializeSharedObject.mockReset()
    mockRepairSharedObject.mockResolvedValue(undefined)
    mockReinitializeSharedObject.mockResolvedValue(undefined)

    mockUseParams.mockReturnValue({ sharedObjectId: SPACE_ID })
    mockUseParentPaths.mockReturnValue([])
    setWatchMocks({ spacesList: [] }, null)
    setResourceMocks(
      {
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      {
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      },
    )
    mockUseResourceValue.mockReturnValue({
      watchResourcesList: vi.fn(),
      watchSharedObjectHealth: vi.fn(),
      spacewave: {
        repairSharedObject: mockRepairSharedObject,
        reinitializeSharedObject: mockReinitializeSharedObject,
      },
    })
    mockSessionUseContext.mockReturnValue({ value: {} })
    mockOrgListUseContextSafe.mockReturnValue({ organizations: [] })
  })

  afterEach(() => {
    cleanup()
  })

  it('redirects legacy /so routes for org-owned spaces', async () => {
    mockOrgListUseContextSafe.mockReturnValue({
      organizations: [{ id: 'org-1', spaceIds: [SPACE_ID] }],
    })

    render(<SessionSharedObjectContainer />)

    await waitFor(() => {
      expect(mockNavigateSession).toHaveBeenCalledWith({
        path: `org/org-1/so/${SPACE_ID}`,
        replace: true,
      })
    })
  })

  it('does not redirect when already nested under an org route', () => {
    mockUseParentPaths.mockReturnValue(['/u/1/org/org-1'])
    mockOrgListUseContextSafe.mockReturnValue({
      organizations: [{ id: 'org-1', spaceIds: [SPACE_ID] }],
    })

    render(<SessionSharedObjectContainer />)

    expect(mockNavigateSession).not.toHaveBeenCalled()
  })

  it('does not redirect when no org claims the space', () => {
    mockOrgListUseContextSafe.mockReturnValue({
      organizations: [{ id: 'org-1', spaceIds: ['other-space'] }],
    })

    render(<SessionSharedObjectContainer />)

    expect(mockNavigateSession).not.toHaveBeenCalled()
  })

  it('renders closed shared object health from the session watch', () => {
    setWatchMocks(
      { spacesList: [] },
      {
        health: {
          status: SharedObjectHealthStatus.CLOSED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
          remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
          error: 'root signature validation failed',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(screen.getByText('Initial state rejected')).toBeTruthy()
    expect(
      screen.getByText(/closed the mount instead of retrying indefinitely/i),
    ).toBeTruthy()
    expect(screen.getByText('root signature validation failed')).toBeTruthy()
  })

  it('renders space loading copy when the object is mounted but the body is still loading', () => {
    setWatchMocks(
      { spacesList: [] },
      {
        health: {
          status: SharedObjectHealthStatus.READY,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.UNKNOWN,
          remediationHint: SharedObjectHealthRemediationHint.NONE,
          error: '',
        },
      },
    )
    setResourceMocks(
      {
        value: { meta: { sharedObjectId: SPACE_ID } },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(screen.getByText('Mounting your space')).toBeTruthy()
    expect(
      screen.getByText('Almost ready. Loading the space contents.'),
    ).toBeTruthy()
  })

  it('renders a body-layer health card for body mount errors', () => {
    setWatchMocks(
      { spacesList: [buildSpaceListEntry('created')] },
      {
        health: {
          status: SharedObjectHealthStatus.READY,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.UNKNOWN,
          remediationHint: SharedObjectHealthRemediationHint.NONE,
          error: '',
        },
      },
    )
    setResourceMocks(
      {
        value: { meta: { sharedObjectId: SPACE_ID } },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      {
        value: null,
        loading: false,
        error: new Error('build cdn world engine: block not found'),
        retry: vi.fn(),
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(screen.getByText('Required block missing')).toBeTruthy()
    expect(screen.getByText(/source data needs repair/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Repair' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Reinitialize' })).toBeTruthy()
  })

  it('renders remediation guidance for revoked access', () => {
    setWatchMocks(
      { spacesList: [] },
      {
        health: {
          status: SharedObjectHealthStatus.CLOSED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
          remediationHint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
          error: 'not a participant',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(screen.getByText('Access revoked')).toBeTruthy()
    expect(
      screen.getByText(/request access again or confirm the correct account/i),
    ).toBeTruthy()
    expect(screen.getByText('not a participant')).toBeTruthy()
  })

  it('renders degraded shared object health distinctly', () => {
    setWatchMocks(
      { spacesList: [buildSpaceListEntry('created')] },
      {
        health: {
          status: SharedObjectHealthStatus.DEGRADED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.UNKNOWN,
          remediationHint: SharedObjectHealthRemediationHint.RETRY,
          error: 'sync is catching up',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(screen.getByText('Shared object degraded')).toBeTruthy()
    expect(
      screen.getByText(
        /partially available, but alpha detected a recoverable problem/i,
      ),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Repair' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Reinitialize' })).toBeTruthy()
  })

  it('disables repair actions for org members without mutation rights', () => {
    mockUseParentPaths.mockReturnValue(['/u/1/org/org-1'])
    mockOrgListUseContextSafe.mockReturnValue({
      loading: false,
      organizations: [
        {
          id: 'org-1',
          role: 'org:member',
          spaceIds: [SPACE_ID],
        },
      ],
    })
    setWatchMocks(
      { spacesList: [buildSpaceListEntry('shared')] },
      {
        health: {
          status: SharedObjectHealthStatus.CLOSED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
          remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
          error: 'root signature validation failed',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(
      screen.getByRole('button', { name: 'Repair' }).hasAttribute('disabled'),
    ).toBe(true)
    expect(
      screen
        .getByRole('button', { name: 'Reinitialize' })
        .hasAttribute('disabled'),
    ).toBe(true)
    expect(
      document.body.textContent?.includes(
        'Only organization owners can repair or reinitialize this shared object.',
      ) ?? false,
    ).toBe(true)
  })

  it('enables repair actions for org owners on broken shared objects', () => {
    mockUseParentPaths.mockReturnValue(['/u/1/org/org-1'])
    mockOrgListUseContextSafe.mockReturnValue({
      loading: false,
      organizations: [
        {
          id: 'org-1',
          role: 'org:owner',
          spaceIds: [SPACE_ID],
        },
      ],
    })
    setWatchMocks(
      { spacesList: [buildSpaceListEntry('shared')] },
      {
        health: {
          status: SharedObjectHealthStatus.CLOSED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
          remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
          error: 'root signature validation failed',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    expect(
      screen.getByRole('button', { name: 'Repair' }).hasAttribute('disabled'),
    ).toBe(false)
    expect(
      screen
        .getByRole('button', { name: 'Reinitialize' })
        .hasAttribute('disabled'),
    ).toBe(false)
  })

  it('distinguishes repair from destructive reinitialize confirmation flow', async () => {
    mockUseParentPaths.mockReturnValue(['/u/1/org/org-1'])
    mockOrgListUseContextSafe.mockReturnValue({
      loading: false,
      organizations: [
        {
          id: 'org-1',
          role: 'org:owner',
          spaceIds: [SPACE_ID],
        },
      ],
    })
    setWatchMocks(
      { spacesList: [buildSpaceListEntry('shared')] },
      {
        health: {
          status: SharedObjectHealthStatus.CLOSED,
          layer: SharedObjectHealthLayer.SHARED_OBJECT,
          commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
          remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
          error: 'root signature validation failed',
        },
      },
    )

    render(<SessionSharedObjectContainer />)

    fireEvent.click(screen.getByRole('button', { name: 'Repair' }))
    expect(document.body.textContent).toContain(
      'Repair is non-destructive. It reuses the normal recovery path and keeps the current shared object identity intact.',
    )
    await waitFor(() => {
      expect(mockRepairSharedObject).toHaveBeenCalledWith(SPACE_ID)
    })

    const reinitializeButton = screen.getByRole('button', {
      name: 'Reinitialize',
    })
    await waitFor(() => {
      expect(reinitializeButton.hasAttribute('disabled')).toBe(false)
    })
    fireEvent.click(reinitializeButton)
    expect(document.body.textContent).toContain('Confirm reinitialize')
    expect(document.body.textContent).toContain(
      'Reinitialize is destructive. It rewrites this shared object in place on the same shared object id and URL.',
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm reinitialize' }),
    )
    await waitFor(() => {
      expect(document.body.textContent).toContain(
        'Reinitialize is selected for this broken shared object.',
      )
      expect(mockReinitializeSharedObject).toHaveBeenCalledWith(SPACE_ID)
    })
  })
})
