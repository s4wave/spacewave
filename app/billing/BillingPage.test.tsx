import type { ButtonHTMLAttributes, PropsWithChildren, ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BillingPage } from './BillingPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockRefreshBillingState = vi.hoisted(() => vi.fn())
const mockSession = vi.hoisted(() => ({
  spacewave: {
    listManagedBillingAccounts: vi.fn().mockResolvedValue({ accounts: [] }),
    refreshBillingState: mockRefreshBillingState,
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

vi.mock('./UsageBars.js', () => ({
  UsageBars: ({ actions }: { actions?: ReactNode }) => (
    <div>
      <div>usage-bars</div>
      {actions}
    </div>
  ),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingInterval: {
    BillingInterval_UNKNOWN: 0,
    BillingInterval_MONTH: 1,
    BillingInterval_YEAR: 2,
  },
  BillingStatus: {
    BillingStatus_NONE: 0,
    BillingStatus_ACTIVE: 2,
    BillingStatus_TRIALING: 3,
    BillingStatus_PAST_DUE: 4,
    BillingStatus_CANCELED: 5,
  },
}))

describe('BillingPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockRefreshBillingState.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('refreshes the billing snapshot when the usage refresh button is clicked', async () => {
    let resolveRefresh: (() => void) | undefined
    mockRefreshBillingState.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveRefresh = resolve
        }),
    )

    render(<BillingPage />)
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() =>
      expect(mockRefreshBillingState).toHaveBeenCalledWith('ba_test'),
    )
    expect(
      screen.getByRole('button', { name: 'Refreshing...' }),
    ).toHaveProperty('disabled', true)

    resolveRefresh?.()

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeDefined(),
    )
  })
})
