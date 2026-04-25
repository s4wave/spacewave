import type { ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BillingAccountDetailRoute } from './BillingAccountDetailRoute.js'
import { BillingAccountsRoute } from './BillingAccountsRoute.js'
import { BillingCancelRoute } from './BillingCancelRoute.js'

const mockUseParams = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useParams: mockUseParams,
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
  afterEach(() => {
    cleanup()
  })

  it('renders the billing account list inside the session frame', () => {
    render(<BillingAccountsRoute />)

    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-accounts-page')).toBeTruthy()
  })

  it('renders the billing detail route inside the session frame', () => {
    mockUseParams.mockReturnValue({ baId: 'ba_detail' })

    render(<BillingAccountDetailRoute />)

    expect(
      screen
        .getByTestId('billing-state-provider')
        .getAttribute('data-billing-account-id'),
    ).toBe('ba_detail')
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-page')).toBeTruthy()
  })

  it('renders the billing cancel route inside the session frame', () => {
    mockUseParams.mockReturnValue({ baId: 'ba_cancel' })

    render(<BillingCancelRoute />)

    expect(
      screen
        .getByTestId('billing-state-provider')
        .getAttribute('data-billing-account-id'),
    ).toBe('ba_cancel')
    expect(screen.getByTestId('session-frame')).toBeTruthy()
    expect(screen.getByTestId('billing-cancel-page')).toBeTruthy()
  })
})
