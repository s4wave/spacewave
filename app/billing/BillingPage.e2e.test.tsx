import type { ButtonHTMLAttributes, PropsWithChildren } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockSession = vi.hoisted(() => ({
  spacewave: {
    listManagedBillingAccounts: vi.fn().mockResolvedValue({ accounts: [] }),
    refreshBillingState: vi.fn(),
    renameBillingAccount: vi.fn(),
  },
}))
const mockBillingState = vi.hoisted(() => ({
  billingAccountId: 'ba_test',
  selfServiceAllowed: true,
  loading: false,
  response: {
    billingAccount: {
      id: 'ba_test',
      displayName: 'Billing Account',
      status: 2,
      billingInterval: 1,
    },
    usage: {
      storageBytes: 1,
      storageBaselineBytes: 10,
      writeOps: 1n,
      writeOpsBaseline: 10n,
      readOps: 1n,
      readOpsBaseline: 10n,
    },
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({ value: mockSession }),
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockSession,
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: () => ({ data: { accounts: [] } }),
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))

vi.mock('@s4wave/web/ui/BackButton.js', () => ({
  BackButton: ({
    children,
    floating: _floating,
    ...props
  }: PropsWithChildren<{ floating?: boolean }>) => (
    <button {...props}>{children}</button>
  ),
}))

vi.mock('@s4wave/web/ui/DashboardButton.js', () => ({
  DashboardButton: (props: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props} />
  ),
}))

vi.mock('./BillingAssignmentsSection.js', () => ({
  BillingAssignmentsSection: () => <div>assignments</div>,
}))

vi.mock('./DeleteBillingAccountSection.js', () => ({
  DeleteBillingAccountSection: () => <div>delete</div>,
}))

vi.mock('./PlanControls.js', () => ({
  PlanControls: () => <div>plan-controls</div>,
}))

vi.mock('./StripePortalLink.js', () => ({
  StripePortalLink: () => <div>portal-link</div>,
}))

import { BillingPage } from './BillingPage.js'

// BillingPageSurface fills the viewport and applies the bg-background-primary
// token so the screenshot captures the billing detail page over its real
// backdrop. UsageBars renders for real off the mocked billing state; the
// heavier action sections are stubbed the same way the unit test stubs them.
function BillingPageSurface() {
  return (
    <div
      data-testid="billing-surface"
      className="bg-background-primary text-foreground fixed inset-0"
    >
      <BillingPage />
    </div>
  )
}

function surfaceBackground(): string {
  const el = document.querySelector('[data-testid="billing-surface"]')
  if (!(el instanceof HTMLElement)) {
    throw new Error('billing surface was not rendered')
  }
  return getComputedStyle(el).backgroundColor
}

async function capture(name: string) {
  return page.screenshot({ path: `__screenshots__/billing/${name}.png` })
}

describe('billing detail browser render', () => {
  it('renders the billing header, status, and usage on a desktop viewport', async () => {
    await render(<BillingPageSurface />)

    await expect
      .element(page.getByRole('heading', { name: 'Billing Account' }))
      .toBeInTheDocument()
    await expect.element(page.getByText('Active')).toBeInTheDocument()
    await expect.element(page.getByText('Usage')).toBeInTheDocument()
    await expect
      .element(page.getByText('Storage', { exact: true }))
      .toBeInTheDocument()

    // The bg-background-primary token must resolve to a real color, proving the
    // app stylesheet loaded rather than falling back to a transparent root.
    expect(surfaceBackground()).not.toBe('rgba(0, 0, 0, 0)')
    expect(surfaceBackground()).not.toBe('transparent')

    await capture('detail-desktop')
    await cleanup()
  })

  it('keeps the billing detail within a narrow viewport without horizontal overflow', async () => {
    await page.viewport(390, 844)

    await render(<BillingPageSurface />)

    await expect
      .element(page.getByRole('heading', { name: 'Billing Account' }))
      .toBeInTheDocument()
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    )

    await capture('detail-narrow')
    await cleanup()
  })
})
