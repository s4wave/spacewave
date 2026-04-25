import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { BillingSummary } from './BillingSummary.js'

const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockBillingState = vi.hoisted(() => ({
  response: null as {
    billingAccount?: {
      id?: string
      status?: number
      billingInterval?: number
      currentPeriodEnd?: bigint
    }
  } | null,
  loading: false,
  selfServiceAllowed: true,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionNavigate: () => mockNavigateSession,
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
  useBillingStateContextSafe: () => mockBillingState,
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
    BillingStatus_PAST_DUE_READONLY: 6,
    BillingStatus_CANCELED: 5,
  },
}))

describe('BillingSummary', () => {
  beforeEach(() => {
    mockNavigateSession.mockReset()
    mockBillingState.response = null
  })

  afterEach(() => {
    cleanup()
  })

  it('renders after billing data arrives on a later render', () => {
    const view = render(<BillingSummary />)
    expect(view.container.firstChild).toBeNull()

    mockBillingState.response = {
      billingAccount: {
        status: 2,
        billingInterval: 1,
        currentPeriodEnd: 1735776000000n,
      },
    }
    view.rerender(<BillingSummary />)

    expect(screen.getByText('Billing')).toBeDefined()
  })

  it('navigates to the billing detail page when the BA id is known', () => {
    mockBillingState.response = {
      billingAccount: {
        id: '01kpmbrht2vspd9fg86sry994c',
        status: 2,
        billingInterval: 1,
      },
    }

    render(<BillingSummary />)
    fireEvent.click(screen.getByRole('button'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'billing/01kpmbrht2vspd9fg86sry994c',
    })
  })

  it('falls back to the billing list when no BA id is set', () => {
    mockBillingState.response = {
      billingAccount: {
        status: 2,
        billingInterval: 1,
      },
    }

    render(<BillingSummary />)
    fireEvent.click(screen.getByRole('button'))

    expect(mockNavigateSession).toHaveBeenCalledWith({ path: 'billing' })
  })
})
