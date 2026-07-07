import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseVisibleQuickstartOptions = vi.hoisted(() => vi.fn())
const mockSetOpenMenu = vi.hoisted(() => vi.fn())
const mockSetStateAtom = vi.hoisted(() => vi.fn())
const mockOpenPathInNewTab = vi.hoisted(() => vi.fn())
const mockClipboard = {
  writeText: vi.fn().mockResolvedValue(undefined),
}

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/app/nav-links.js', () => ({
  useNavLinks: () => ({
    blog: vi.fn(),
    changelog: vi.fn(),
    community: vi.fn(),
    docs: vi.fn(),
    download: vi.fn(),
    legal: vi.fn(),
    support: vi.fn(),
  }),
}))

vi.mock('@s4wave/app/quickstart/QuickstartCommands.js', () => ({
  QuickstartCommands: () => null,
}))

vi.mock('@s4wave/app/quickstart/useQuickstartOptions.js', () => ({
  useVisibleQuickstartOptions: mockUseVisibleQuickstartOptions,
}))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useSessionOnboardingState: () => ({
    onboarding: {
      backupComplete: true,
      lockComplete: true,
    },
    markBackupComplete: vi.fn(),
    markLockComplete: vi.fn(),
  }),
}))

vi.mock('@s4wave/app/ShellTabContext.js', () => ({
  useShellTabs: () => ({
    activeTabId: 'home',
    openPathInNewTab: mockOpenPathInNewTab,
  }),
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mockSetOpenMenu,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['session-settings'],
  useStateAtom: () => ['/', mockSetStateAtom],
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/ui/loading/LoadingInline.js', () => ({
  LoadingInline: ({ label }: { label: string }) => <span>{label}</span>,
}))

import { SessionDashboard } from './SessionDashboard.js'

function QuickstartIcon({ className }: { className?: string }) {
  return <svg className={className} aria-hidden="true" />
}

function expectTextBefore(first: string, second: string) {
  const text = document.body.textContent ?? ''
  expect(text.indexOf(first)).toBeGreaterThanOrEqual(0)
  expect(text.indexOf(second)).toBeGreaterThanOrEqual(0)
  expect(text.indexOf(first)).toBeLessThan(text.indexOf(second))
}

describe('SessionDashboard', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockSetOpenMenu.mockReset()
    mockSetStateAtom.mockReset()
    mockOpenPathInNewTab.mockReset()
    mockUseVisibleQuickstartOptions.mockReset()
    Object.defineProperty(navigator, 'clipboard', {
      value: mockClipboard,
      writable: true,
      configurable: true,
    })
    mockClipboard.writeText.mockClear()
    mockUseVisibleQuickstartOptions.mockReturnValue([
      {
        id: 'account',
        name: 'Sign in or create account',
        description: 'Access your account',
        category: 'account',
        icon: QuickstartIcon,
        path: '/login',
      },
      {
        id: 'pair',
        name: 'Enter a device pairing code',
        description: 'Link to an existing device via pairing code',
        category: 'account',
        icon: QuickstartIcon,
        path: '/pair',
      },
      {
        id: 'space',
        name: 'Create an Empty Space',
        description: 'Start with a blank space',
        category: 'storage',
        icon: QuickstartIcon,
      },
      {
        id: 'drive',
        name: 'Create a Drive',
        description: 'Drive workspace',
        category: 'storage',
        icon: QuickstartIcon,
      },
      {
        id: 'canvas',
        name: 'Create a Canvas',
        description: 'Visual workspace',
        category: 'storage',
        icon: QuickstartIcon,
      },
    ])
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('promotes Drive as the empty-dashboard next action before joining or browsing templates', () => {
    render(<SessionDashboard spaces={[]} onQuickstartClick={vi.fn()} />)

    expect(screen.getByText('Continue')).toBeDefined()
    expect(screen.getByText('Other starts')).toBeDefined()
    expect(screen.getByText('Browse templates')).toBeDefined()
    expectTextBefore('Create a Drive', 'Join Space')
    expectTextBefore('Create a Drive', 'Create a Canvas')
  })

  it('keeps returning sessions focused on existing spaces before create actions', () => {
    render(
      <SessionDashboard
        spaces={[{ id: 'space-1', name: 'Alpha Space' }]}
        onQuickstartClick={vi.fn()}
      />,
    )

    expect(screen.queryByText('Continue')).toBeNull()
    expectTextBefore('Alpha Space', 'Join Space')
    expectTextBefore('Join Space', 'Create a Drive')
  })

  it('labels existing spaces by human source and copies the full space ID from the affordance', () => {
    const ownedSpaceId = 'space-owned-01HZY4PZQWVN9A1K4J4Y7P6C3R'
    const sharedSpaceId = 'space-shared-01HZY4Q7P4CQVWDTVR9N6DJ6ZY'

    render(
      <SessionDashboard
        spaces={[
          {
            id: ownedSpaceId,
            name: 'Alpha Space',
            source: 'created',
          },
          {
            id: sharedSpaceId,
            name: 'Beta Space',
            source: 'shared',
          },
        ]}
        onQuickstartClick={vi.fn()}
      />,
    )

    const ownedSubtitle = screen.getByText('Owned Space')
    const sharedSubtitle = screen.getByText('Shared Space')
    expect(ownedSubtitle.textContent).not.toContain(ownedSpaceId)
    expect(sharedSubtitle.textContent).not.toContain(sharedSpaceId)

    fireEvent.click(screen.getByRole('button', { name: 'Copy Alpha Space ID' }))
    expect(mockClipboard.writeText).toHaveBeenCalledWith(ownedSpaceId)

    fireEvent.click(screen.getByRole('button', { name: 'Copy Beta Space ID' }))
    expect(mockClipboard.writeText).toHaveBeenCalledWith(sharedSpaceId)
  })

  it('does not promote creation actions for read-only sessions', () => {
    render(
      <SessionDashboard spaces={[]} readOnly onQuickstartClick={vi.fn()} />,
    )

    expect(screen.queryByText('Continue')).toBeNull()
    expect(screen.queryByText('Create a Drive')).toBeNull()
    expect(screen.getByText('Other starts')).toBeDefined()
    expect(screen.getByText('Join Space')).toBeDefined()
  })

  it('opens Docs through provider Shell Tab semantics', () => {
    render(<SessionDashboard spaces={[]} onQuickstartClick={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    expect(mockOpenPathInNewTab).toHaveBeenCalledWith('/docs', {
      afterTabId: 'home',
      focusExisting: true,
    })
  })
})
