import { beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseVisibleQuickstartOptions = vi.hoisted(() => vi.fn())
const mockSetOpenMenu = vi.hoisted(() => vi.fn())
const mockSetStateAtom = vi.hoisted(() => vi.fn())
const mockOpenPathInNewTab = vi.hoisted(() => vi.fn())

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

import { SessionDashboard } from './SessionDashboard.js'

function QuickstartIcon({ className }: { className?: string }) {
  return <svg className={className} aria-hidden="true" />
}

const quickstartOptions = [
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
]

// SessionDashboardSurface fills the viewport so the dashboard's h-full column
// resolves, and applies the bg-background-landing token so the screenshot
// captures the real dashboard backdrop rather than a transparent root.
function SessionDashboardSurface() {
  return (
    <div
      data-testid="dashboard-surface"
      className="bg-background-landing text-foreground fixed inset-0"
    >
      <SessionDashboard spaces={[]} onQuickstartClick={vi.fn()} />
    </div>
  )
}

function surfaceBackground(): string {
  const el = document.querySelector('[data-testid="dashboard-surface"]')
  if (!(el instanceof HTMLElement)) {
    throw new Error('dashboard surface was not rendered')
  }
  return getComputedStyle(el).backgroundColor
}

async function capture(name: string) {
  return page.screenshot({
    path: `__screenshots__/session-dashboard/${name}.png`,
  })
}

describe('session dashboard browser render', () => {
  beforeEach(() => {
    mockUseVisibleQuickstartOptions.mockReturnValue(quickstartOptions)
  })

  it('renders the empty-session get-started palette on a desktop viewport', async () => {
    await render(<SessionDashboardSurface />)

    await expect
      .element(page.getByPlaceholder('Get started...'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Create a Drive')).toBeInTheDocument()
    await expect
      .element(page.getByText('Secure Your Account'))
      .toBeInTheDocument()

    // The bg-background-landing token must resolve to a real color, proving the
    // app stylesheet loaded rather than falling back to a transparent root.
    expect(surfaceBackground()).not.toBe('rgba(0, 0, 0, 0)')
    expect(surfaceBackground()).not.toBe('transparent')

    await capture('empty-desktop')
    await cleanup()
  })

  it('keeps the dashboard within a narrow viewport without horizontal overflow', async () => {
    await page.viewport(390, 844)

    await render(<SessionDashboardSurface />)

    await expect
      .element(page.getByPlaceholder('Get started...'))
      .toBeInTheDocument()
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    )

    await capture('empty-narrow')
    await cleanup()
  })
})
