import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

const mockUseWatchStateRpc = vi.hoisted(() => vi.fn())
const mockActiveRoute = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: mockUseWatchStateRpc,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          watchOrganizationState: vi.fn(),
        },
      },
    }),
  },
}))

vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: {
    useContextSafe: () => ({
      loading: false,
      organizations: [
        {
          id: 'org-1',
          displayName: 'Studio',
          role: 'org:owner',
          spaceIds: ['space-1'],
        },
      ],
    }),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({ orgId: 'org-1', sharedObjectId: 'space-1' }),
  useParentPaths: () => ['/u/1/org/org-1'],
  usePath: () => '/u/1/org/org-1',
  Routes: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
  Route: ({ path, children }: { path: string; children?: React.ReactNode }) =>
    path === mockActiveRoute() ? <>{children}</> : null,
}))

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="bottom-bar-level">{children}</div>
  ),
}))

vi.mock('@s4wave/web/frame/bottom-bar-item.js', () => ({
  BottomBarItem: ({ children }: { children?: React.ReactNode }) => (
    <button>{children}</button>
  ),
}))

vi.mock('@s4wave/web/frame/bottom-icon-props.js', () => ({
  bottomBarIconProps: {},
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => vi.fn(),
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...values: Array<string | false | null | undefined>) =>
    values.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/app/session/SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

vi.mock('@s4wave/app/session/SessionSharedObjectContainer.js', () => ({
  SessionSharedObjectContainer: () => <div data-testid="shared-object-route" />,
}))

vi.mock('./OrganizationDashboard.js', () => ({
  OrganizationDashboard: () => <div data-testid="org-dashboard" />,
}))

vi.mock('./OrganizationDetails.js', () => ({
  OrganizationDetails: () => <div data-testid="org-details" />,
}))

import { OrgContainer } from './OrgContainer.js'

describe('OrgContainer', () => {
  beforeEach(() => {
    mockUseWatchStateRpc.mockReset()
    mockUseWatchStateRpc.mockReturnValue(null)
    mockActiveRoute.mockReset()
    mockActiveRoute.mockReturnValue('*')
  })

  afterEach(() => {
    cleanup()
  })

  it('keeps the org dashboard mounted when org state is unavailable', () => {
    render(<OrgContainer />)

    expect(screen.getByTestId('org-dashboard')).toBeDefined()
    expect(screen.queryByText('Loading organization...')).toBeNull()
  })

  it('keeps the canonical nested shared-object route mounted while the org root is degraded', () => {
    mockActiveRoute.mockReturnValue('/so/:sharedObjectId/*')

    render(<OrgContainer />)

    expect(screen.getByTestId('shared-object-route')).toBeDefined()
    expect(screen.queryByText('Loading organization...')).toBeNull()
  })
})
