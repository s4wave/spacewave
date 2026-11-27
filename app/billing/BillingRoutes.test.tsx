import type { ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BillingAccountDetailRoute } from './BillingAccountDetailRoute.js'
import { BillingAccountsRoute } from './BillingAccountsRoute.js'
import { BillingCancelRoute } from './BillingCancelRoute.js'

const mockUseParams = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useParams: mockUseParams,
  useNavigate: () => vi.fn(),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/app/session/SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

vi.mock('./BillingAccountsPage.js', () => ({
  BillingAccountsPage: () => <div data-testid="billing-accounts-page" />,
}))

vi.mock('./BillingPage.js', () => ({
  BillingPage: () => <div data-testid="billing-page" />,
}))

vi.mock('./BillingCancelPage.js', () => ({
  BillingCancelPage: () => <div data-testid="billing-cancel-page" />,
}))

vi.mock('./BillingStateProvider.js', () => ({
  BillingStateProvider: ({
    billingAccountId,
    children,
  }: {
    billingAccountId: string
    children?: ReactNode
  }) => (
    <div
      data-testid="billing-state-provider"
      data-billing-account-id={billingAccountId}
    >
      {children}
    </div>
  ),
}))

describe('billing session routes', () => {
  beforeEach(() => {
    // The account-list route belongs to a loaded Cloud session by default.
    mockUseSessionInfo.mockReturnValue({
      providerId: 'spacewave',
      isCloud: true,
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('renders the billing account list inside the session frame', () => {
    render(<BillingAccountsRoute />)

    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-accounts-page')).toBeTruthy()
  })

  it('explains local billing inside the session frame', () => {
    // Local sessions never open the Cloud account list.
    mockUseSessionInfo.mockReturnValue({ providerId: 'local', isCloud: false })
    render(<BillingAccountsRoute />)

    // Keep the session frame and show the local-session explanation.
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByText('No billing on a local session')).toBeTruthy()
    expect(screen.queryByTestId('billing-accounts-page')).toBeNull()
  })

  it('waits for the session provider before opening billing accounts', () => {
    // An unresolved provider is distinct from a loaded local session.
    mockUseSessionInfo.mockReturnValue({ providerId: '', isCloud: false })
    render(<BillingAccountsRoute />)

    // Show the pending state until the session can choose its billing route.
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByText('Loading billing accounts')).toBeTruthy()
    expect(screen.queryByTestId('billing-accounts-page')).toBeNull()
    expect(screen.queryByText('No billing on a local session')).toBeNull()
  })

  it('renders the billing detail route inside the session frame', () => {
    // Select the account specified by the detail route.
    mockUseParams.mockReturnValue({ baId: 'ba_detail' })

    // Mount the account detail route through its session frame.
    render(<BillingAccountDetailRoute />)

    // Keep the selected account, frame and detail page together.
    expect(
      screen
        .getByTestId('billing-state-provider')
        .getAttribute('data-billing-account-id'),
    ).toBe('ba_detail')
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-page')).toBeTruthy()
  })

  it('renders the billing cancel route inside the session frame', () => {
    // Select the account specified by the cancellation route.
    mockUseParams.mockReturnValue({ baId: 'ba_cancel' })

    // Mount the cancellation route through its session frame.
    render(<BillingCancelRoute />)

    // Keep the selected account, frame and cancellation page together.
    expect(
      screen
        .getByTestId('billing-state-provider')
        .getAttribute('data-billing-account-id'),
    ).toBe('ba_cancel')
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-cancel-page')).toBeTruthy()
  })
})
