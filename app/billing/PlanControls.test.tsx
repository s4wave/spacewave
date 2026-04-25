import type { ButtonHTMLAttributes } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PlanControls } from './PlanControls.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockReactivateSubscription = vi.hoisted(() => vi.fn())
const mockStartCheckout = vi.hoisted(() => vi.fn())
const mockBillingState = vi.hoisted(() => ({
  billingAccountId: 'ba_test',
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          reactivateSubscription: mockReactivateSubscription,
        },
      },
    }),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('../provider/spacewave/useBillingAccountCheckout.js', () => ({
  useBillingAccountCheckout: () => ({
    continueCheckout: vi.fn(),
    error: null,
    polling: false,
    showRetry: false,
    startCheckout: mockStartCheckout,
  }),
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))

vi.mock('@s4wave/web/ui/DashboardButton.js', () => ({
  DashboardButton: (props: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props} />
  ),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingStatus: {
    BillingStatus_ACTIVE: 2,
    BillingStatus_TRIALING: 3,
    BillingStatus_CANCELED: 5,
  },
}))

describe('PlanControls', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockReactivateSubscription.mockReset()
    mockStartCheckout.mockReset()
    window.location.hash = '#/u/1/billing/ba_test'
  })

  afterEach(() => {
    cleanup()
  })

  it('starts checkout for the same billing account when reactivation needs checkout', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })

    render(<PlanControls status={5} showSelfService={true} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith('ba_test'),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('auto-starts reactivation once when the billing page carries a reactivate intent', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })
    window.location.hash = '#/u/1/billing/ba_test?reactivate=1'

    render(<PlanControls status={5} showSelfService={true} />)

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith('ba_test'),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
    expect(window.location.hash).toBe('#/u/1/billing/ba_test')
  })
})
