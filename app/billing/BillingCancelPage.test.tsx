import type { ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BillingCancelPage } from './BillingCancelPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockReactivateSubscription = vi.hoisted(() => vi.fn())
const mockStartCheckout = vi.hoisted(() => vi.fn())
const mockCheckout = vi.hoisted(() => ({
  continueCheckout: vi.fn(),
  error: null as string | null,
  polling: false,
  showRetry: false,
  startCheckout: mockStartCheckout,
}))
let lastCheckoutOptions: { onCompleted?: () => void } | undefined
const mockBillingState = vi.hoisted(() => ({
  billingAccountId: 'ba_test',
  response: {
    billingAccount: {
      lifecycleState: 2,
      cancelAt: 1735776000000n,
    },
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          reactivateSubscription: mockReactivateSubscription,
          cancelSubscription: vi.fn(),
        },
      },
    }),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('../provider/spacewave/useBillingAccountCheckout.js', () => ({
  useBillingAccountCheckout: (opts?: { onCompleted?: () => void }) => {
    lastCheckoutOptions = opts
    return mockCheckout
  },
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))

vi.mock('@s4wave/app/provider/spacewave/CloudConfirmationPage.js', () => ({
  PageWrapper: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  FaqAccordion: () => null,
  PageFooter: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => null,
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...parts: Array<string | false | null | undefined>) =>
    parts.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  AccountLifecycleState: {
    AccountLifecycleState_ACTIVE: 1,
    AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END: 2,
    AccountLifecycleState_CANCELED_GRACE_READONLY: 3,
  },
}))

describe('BillingCancelPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockReactivateSubscription.mockReset()
    mockStartCheckout.mockReset()
    mockCheckout.continueCheckout.mockReset()
    mockCheckout.error = null
    mockCheckout.polling = false
    mockCheckout.showRetry = false
    lastCheckoutOptions = undefined
  })

  afterEach(() => {
    cleanup()
  })

  it('starts checkout for the same billing account when reactivation needs checkout', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })

    render(<BillingCancelPage />)
    fireEvent.click(
      screen.getByRole('button', { name: /keep subscription active/i }),
    )

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith('ba_test'),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
  })

  it('returns to billing detail after checkout confirmation', () => {
    render(<BillingCancelPage />)

    lastCheckoutOptions?.onCompleted?.()

    expect(mockNavigate).toHaveBeenCalledWith({ path: '../' })
  })
})
