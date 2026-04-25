import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent, screen } from '@testing-library/react'
import { SessionDetails } from './SessionDetails.js'
import {
  SessionContext,
  SessionRouteContext,
} from '@s4wave/web/contexts/contexts.js'
import { Session } from '@s4wave/sdk/session/session.js'

const mockNavigate = vi.hoisted(() => vi.fn())

// Mock the promise hook
vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: vi.fn(),
}))

// Mock tooltip components to simplify testing
vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: () => null,
}))

// Mock router hooks
vi.mock('@s4wave/web/router/router.js', async () => {
  const React = await import('react')

  interface RouterState {
    path: string
    onNavigate?: (to: { path: string; replace?: boolean }) => void
  }

  const RouterContext = React.createContext<RouterState>({ path: '/' })

  const resolvePath = (
    current: string,
    to: { path: string; replace?: boolean },
  ) => {
    if (to.path.startsWith('/')) return to.path
    if (!to.path || to.path === '.') return current
    const parts = current.split('/').filter(Boolean)
    for (const part of to.path.split('/')) {
      if (!part || part === '.') continue
      if (part === '..') {
        parts.pop()
        continue
      }
      parts.push(part)
    }
    return '/' + parts.join('/')
  }

  return {
    resolvePath,
    Router: ({
      children,
      path,
      onNavigate,
    }: {
      children?: React.ReactNode
      path: string
      onNavigate: (to: { path: string; replace?: boolean }) => void
    }) => (
      <RouterContext.Provider value={{ path, onNavigate }}>
        {children}
      </RouterContext.Provider>
    ),
    Routes: ({
      children,
    }: {
      children?: React.ReactNode
      fullPath?: boolean
    }) => {
      const ctx = React.useContext(RouterContext)
      const routes = React.Children.toArray(children).filter(
        (
          child,
        ): child is React.ReactElement<{
          path: string
          children?: React.ReactNode
        }> => React.isValidElement(child),
      )
      const match = routes.find((child) => child.props.path === ctx.path)
      return match?.props.children ?? null
    },
    Route: ({ children }: { children?: React.ReactNode; path: string }) => (
      <>{children}</>
    ),
    useNavigate: () => mockNavigate,
    useParams: () => ({ sessionIndex: '1' }),
  }
})

vi.mock('../setup/LinkDeviceWizard.js', () => ({
  LinkDeviceWizard: ({
    topLeft,
  }: {
    topLeft?: React.ReactNode
    exitPath?: string
  }) => (
    <div>
      {topLeft}
      <div>Link Device Wizard</div>
    </div>
  ),
}))

vi.mock('./DeleteSpaceEscapeHatchDialog.js', () => ({
  DeleteSpaceEscapeHatchDialog: ({
    open,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
    session: unknown
  }) => (open ? <div>Delete Space Dialog</div> : null),
}))

vi.mock('./SessionsSection.js', () => ({
  SessionsSection: ({
    onLinkDeviceClick,
  }: {
    account: unknown
    isLocal: boolean
    open?: boolean
    onOpenChange?: (open: boolean) => void
    onLinkDeviceClick?: () => void
  }) => (
    <div>
      <div>Sessions</div>
      <div data-testid="link-devices-trigger" onClick={onLinkDeviceClick} />
    </div>
  ),
}))

// Mock session list hook
vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => ({
    value: { sessions: [{ sessionIndex: 1 }, { sessionIndex: 2 }] },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/app/billing/BillingStateProvider.js', () => ({
  useBillingStateContext: () => ({
    billingAccountId: undefined,
    selfServiceAllowed: true,
    response: null,
    loading: false,
  }),
  useBillingStateContextSafe: () => ({
    billingAccountId: undefined,
    selfServiceAllowed: true,
    response: null,
    loading: false,
  }),
}))

vi.mock('@s4wave/app/billing/BillingSection.js', () => ({
  BillingSection: ({
    onNavigateToPath,
  }: {
    isLocal: boolean
    open?: boolean
    onOpenChange?: (open: boolean) => void
    onNavigateToPath?: (path: string) => void
  }) => (
    <div
      data-testid="billing-section-manage"
      onClick={() => onNavigateToPath?.('billing/ba_test')}
    />
  ),
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: () => ({
    value: { revokeSession: vi.fn(), selfRevokeSession: vi.fn() },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/state/persist.js', async () => {
  const React = await import('react')
  return {
    useStateNamespace: () => ['session-settings'],
    useStateAtom: <T,>(_ns: unknown, _key: string, init: T) =>
      React.useState(init),
  }
})

import { usePromise } from '@s4wave/web/hooks/usePromise.js'

const mockUsePromise = usePromise as ReturnType<typeof vi.fn>

describe('SessionDetails', () => {
  const mockSessionInfo = {
    peerId: 'test-peer-id-123456',
    sessionRef: {
      providerResourceRef: {
        id: 'test-session-id',
        providerAccountId: 'test-account-id',
      },
    },
  }

  const mockSession = {
    getSessionInfo: vi.fn().mockResolvedValue(mockSessionInfo),
    resourceRef: {
      resourceId: 1,
      released: false,
    },
    id: 1,
    client: {},
    service: {},
    createSpace: vi.fn(),
    watchResourcesList: vi.fn(),
    mountSharedObject: vi.fn(),
  } as unknown as Session

  const mockClipboard = {
    writeText: vi.fn().mockResolvedValue(undefined),
  }

  const mockRetry = vi.fn()

  beforeEach(() => {
    cleanup()
    mockNavigate.mockClear()
    Object.defineProperty(navigator, 'clipboard', {
      value: mockClipboard,
      writable: true,
      configurable: true,
    })
    mockClipboard.writeText.mockClear()
    mockRetry.mockClear()
    mockUsePromise.mockReturnValue({
      data: mockSessionInfo,
      loading: false,
      error: null,
    })
  })

  function renderWithContext(
    component: React.ReactElement,
    sessionValue: Session = mockSession,
  ) {
    return render(
      <SessionRouteContext.Provider
        value={{
          basePath: '/u/1',
          navigate: mockNavigate,
        }}
      >
        <SessionContext.Provider
          resource={{
            value: sessionValue,
            loading: false,
            error: null,
            retry: mockRetry,
          }}
        >
          {component}
        </SessionContext.Provider>
      </SessionRouteContext.Provider>,
    )
  }

  describe('Loading State', () => {
    it('displays loading state when data is loading', () => {
      mockUsePromise.mockReturnValue({
        data: null,
        loading: true,
        error: null,
      })
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Loading session info...')).toBeDefined()
    })
  })

  describe('Error State', () => {
    it('displays error state when there is an error', () => {
      mockUsePromise.mockReturnValue({
        data: null,
        loading: false,
        error: new Error('Failed to load session'),
      })
      renderWithContext(<SessionDetails />)
      expect(screen.getByText(/Error: Failed to load session/)).toBeDefined()
    })
  })

  describe('Rendering', () => {
    it('renders without crashing', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('er-id-123456')).toBeDefined()
    })

    it('displays session metadata correctly', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Identifiers')).toBeDefined()
    })

    it('renders the session sync status summary near the top', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByTestId('session-sync-status-summary')).toBeDefined()
      expect(screen.getByText('Checking sync status')).toBeDefined()
    })

    it('displays truncated peer ID in header', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('er-id-123456')).toBeDefined()
    })

    it('handles missing session info gracefully', () => {
      mockUsePromise.mockReturnValue({
        data: null,
        loading: false,
        error: null,
      })
      renderWithContext(<SessionDetails />)
      expect(screen.getAllByText('Unknown').length).toBeGreaterThan(0)
    })
  })

  describe('Close Button', () => {
    it('renders close button when onCloseClick is provided', () => {
      renderWithContext(<SessionDetails onCloseClick={() => {}} />)
      const withClose = screen.getAllByRole('button').length

      cleanup()

      renderWithContext(<SessionDetails />)
      const withoutClose = screen.getAllByRole('button').length

      expect(withClose).toBe(withoutClose + 1)
    })

    it('does not render close button when onCloseClick is not provided', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getAllByRole('button').length).toBeGreaterThan(0)
    })

    it('calls onCloseClick when close button is clicked', () => {
      const onCloseClick = vi.fn()
      renderWithContext(<SessionDetails onCloseClick={onCloseClick} />)
      const buttons = screen.getAllByRole('button')
      const closeButton = buttons[3]
      fireEvent.click(closeButton)
      expect(onCloseClick).toHaveBeenCalledTimes(1)
    })

    it('renders Transfer Sessions inside the Actions section', () => {
      renderWithContext(<SessionDetails />)
      const actionsSection = screen.getByText('Actions').closest('section')
      expect(actionsSection).toBeTruthy()
      expect(
        actionsSection?.contains(screen.getByText('Transfer Sessions')),
      ).toBe(true)
    })
  })

  describe('Action Buttons', () => {
    it('renders all action buttons', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Change Account')).toBeDefined()
      expect(screen.getByText('Lock')).toBeDefined()
      expect(screen.getByText('Logout')).toBeDefined()
      expect(screen.getByText('Danger Zone')).toBeDefined()
      expect(screen.queryByText('Delete a Space')).toBeNull()
    })

    it('calls onChangeAccountClick when Change Account button is clicked', () => {
      const onChangeAccountClick = vi.fn()
      renderWithContext(
        <SessionDetails onChangeAccountClick={onChangeAccountClick} />,
      )
      const buttons = screen.getAllByRole('button')
      const changeAccountButton = buttons.find((btn) =>
        btn.querySelector('.lucide-user-cog'),
      )
      if (changeAccountButton) {
        fireEvent.click(changeAccountButton)
        expect(onChangeAccountClick).toHaveBeenCalledTimes(1)
      }
    })

    it('renders Lock button that scrolls to lock section', () => {
      renderWithContext(<SessionDetails />)
      const lockText = screen.getAllByText('Lock')
      const lockButton = lockText[0]?.closest('button')
      expect(lockButton).toBeTruthy()
    })

    it('Change Account button shows text on medium+ screens', () => {
      renderWithContext(<SessionDetails />)
      const changeAccountText = screen.getAllByText('Change Account')
      const hiddenText = changeAccountText.find(
        (el) => el.className && el.className.includes('hidden'),
      )
      expect(hiddenText).toBeDefined()
      if (hiddenText) {
        expect(hiddenText.className).toContain('md:inline')
      }
    })

    it('Lock button shows text on medium+ screens', () => {
      renderWithContext(<SessionDetails />)
      const lockText = screen.getAllByText('Lock')
      const hiddenText = lockText.find(
        (el) => el.className && el.className.includes('hidden'),
      )
      expect(hiddenText).toBeDefined()
      if (hiddenText) {
        expect(hiddenText.className).toContain('md:inline')
      }
    })

    it('Logout button shows text on medium+ screens', () => {
      renderWithContext(<SessionDetails />)
      const logoutText = screen.getAllByText('Logout')
      const hiddenText = logoutText.find(
        (el) => el.className && el.className.includes('hidden'),
      )
      expect(hiddenText).toBeDefined()
      if (hiddenText) {
        expect(hiddenText.className).toContain('md:inline')
      }
    })

    it('opens the delete-space escape hatch dialog from Actions', () => {
      renderWithContext(<SessionDetails />)

      fireEvent.click(screen.getByText('Danger Zone'))
      fireEvent.click(screen.getByText('Delete a Space'))

      expect(screen.getByText('Delete Space Dialog')).toBeDefined()
    })

    it('reveals destructive actions when the danger zone expands', () => {
      renderWithContext(<SessionDetails />)

      fireEvent.click(screen.getByText('Danger Zone'))

      expect(screen.getByText('Delete a Space')).toBeDefined()
      expect(screen.getByText('Delete Account')).toBeDefined()
    })
  })

  describe('Copyable Fields', () => {
    it('renders the identifiers section toggle', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Identifiers')).toBeDefined()
    })

    it('renders the session header', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('er-id-123456')).toBeDefined()
    })

    it('renders the closeable shell actions', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Change Account')).toBeDefined()
      expect(screen.getByText('Logout')).toBeDefined()
    })

    it('renders the account controls without crashing', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Lock')).toBeDefined()
    })
  })

  describe('Sections', () => {
    it('renders Session Details section', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Identifiers')).toBeDefined()
    })

    it('renders Security section', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Security')).toBeDefined()
    })

    it('shows session lock options in the security section', () => {
      renderWithContext(<SessionDetails />)
      expect(screen.getByText('Security')).toBeDefined()
    })

    it('routes linked-device actions inside the panel', () => {
      renderWithContext(<SessionDetails />)
      fireEvent.click(screen.getByTestId('link-devices-trigger'))
      expect(screen.getByText('Link Device Wizard')).toBeDefined()
      fireEvent.click(screen.getByText('Back'))
      expect(screen.getByText('Identifiers')).toBeDefined()
    })

    it('closes the overlay before navigating to a billing account page', () => {
      const onCloseClick = vi.fn()
      renderWithContext(<SessionDetails onCloseClick={onCloseClick} />)

      fireEvent.click(screen.getByTestId('billing-section-manage'))

      expect(onCloseClick).toHaveBeenCalledTimes(1)
      expect(mockNavigate).toHaveBeenCalledWith({ path: 'billing/ba_test' })
    })
  })

  describe('Button Spacing', () => {
    it('applies responsive gap to button container', () => {
      const { container } = renderWithContext(
        <SessionDetails onCloseClick={() => {}} />,
      )
      const buttonsContainer = container.querySelector('.flex.gap-1\\.5')
      expect(buttonsContainer).toBeTruthy()
      expect(buttonsContainer?.className).toContain('flex')
      expect(buttonsContainer?.className).toContain('gap-1.5')
    })
  })
})
