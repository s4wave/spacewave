import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'

import { NoActiveBillingAccountPage } from './NoActiveBillingAccountPage.js'

const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockUsePromise = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockUseStreamingResource = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUseSessionMetadata = vi.hoisted(() => vi.fn())
const mockUseSessionIndex = vi.hoisted(() => vi.fn(() => 1))
const mockOrgListUseContextSafe = vi.hoisted(() => vi.fn(() => null))
const mockUseCloudProviderConfig = vi.hoisted(() =>
  vi.fn(() => ({
    accountBaseUrl: 'https://account.spacewave.example',
    publicBaseUrl: 'https://spacewave.example',
  })),
)
const mockSessionResource = vi.hoisted(() => ({
  value: null as unknown,
  loading: false,
  error: null,
  retry: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
  useSessionNavigate: () => mockNavigateSession,
  useSessionIndex: mockUseSessionIndex,
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: mockUsePromise,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: mockUseStreamingResource,
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: {
    useContextSafe: mockOrgListUseContextSafe,
  },
}))

vi.mock('./useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: mockUseCloudProviderConfig,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('./CloudConfirmationPage.js', () => ({
  PageWrapper: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  PageFooter: () => <div data-testid="page-footer" />,
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: ({ to }: { to: string }) => <div data-testid="redirect">{to}</div>,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  AccountLifecycleState: {
    AccountLifecycleState_ACTIVE: 'active',
    AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END:
      'active_with_cancel_at_period_end',
    AccountLifecycleState_CANCELED_GRACE_READONLY: 'canceled_grace_readonly',
    AccountLifecycleState_LAPSED_READONLY: 'lapsed_readonly',
    AccountLifecycleState_PENDING_DELETE_READONLY: 'pending_delete_readonly',
    AccountLifecycleState_DELETED_PENDING_PURGE: 'deleted_pending_purge',
    AccountLifecycleState_DELETED: 'deleted',
  },
  BillingStatus: {
    BillingStatus_NONE: 'none',
    BillingStatus_ACTIVE: 'active',
    BillingStatus_TRIALING: 'trialing',
  },
  CheckoutStatus: {
    CheckoutStatus_UNKNOWN: 0,
    CheckoutStatus_PENDING: 1,
    CheckoutStatus_COMPLETED: 2,
    CheckoutStatus_EXPIRED: 3,
    CheckoutStatus_CANCELED: 4,
  },
}))

async function flushAsync() {
  await act(async () => {})
  await act(async () => {})
}

describe('NoActiveBillingAccountPage', () => {
  const mockCreateCheckoutSession = vi.fn()
  const mockCreateBillingAccount = vi.fn()
  const mockAssignBillingAccount = vi.fn()
  const mockWatchCheckoutStatus = vi.fn()
  const mockSession = {
    spacewave: {
      listManagedBillingAccounts: vi.fn(),
      createCheckoutSession: mockCreateCheckoutSession,
      createBillingAccount: mockCreateBillingAccount,
      assignBillingAccount: mockAssignBillingAccount,
      watchCheckoutStatus: mockWatchCheckoutStatus,
    },
  }
  let streamingValue: { status?: number } | null

  beforeEach(() => {
    cleanup()
    mockNavigateSession.mockReset()
    mockUsePromise.mockReset()
    mockUseResourceValue.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionInfo.mockReset()
    mockUseSessionMetadata.mockReset()
    mockUseSessionIndex.mockReset()
    mockOrgListUseContextSafe.mockReset()
    mockUseCloudProviderConfig.mockReset()
    mockCreateCheckoutSession.mockReset()
    mockCreateBillingAccount.mockReset()
    mockAssignBillingAccount.mockReset()
    mockWatchCheckoutStatus.mockReset()

    streamingValue = null
    mockUseResourceValue.mockReturnValue(mockSession)
    mockUseSessionInfo.mockReturnValue({ accountId: 'acct_1' })
    mockUseSessionMetadata.mockReturnValue({ displayName: 'Casey' })
    mockUseSessionIndex.mockReturnValue(1)
    mockOrgListUseContextSafe.mockReturnValue(null)
    mockUseCloudProviderConfig.mockReturnValue({
      accountBaseUrl: 'https://account.spacewave.example',
      publicBaseUrl: 'https://spacewave.example',
    })
    mockUsePromise.mockReturnValue({
      data: {
        accounts: [
          {
            id: 'ba_1',
            displayName: 'Billing Account',
            subscriptionStatus: 'canceled',
            lifecycleState: 'active',
          },
        ],
      },
      loading: false,
      error: null,
    })
    mockUseStreamingResource.mockImplementation(() => ({
      value: streamingValue,
      loading: false,
      error: null,
      retry: vi.fn(),
    }))
    vi.spyOn(window, 'open').mockReturnValue({} as Window)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('redirects to setup when checkout completes immediately', async () => {
    mockCreateCheckoutSession.mockResolvedValue({
      checkoutUrl: '',
      status: 2,
    })

    render(<NoActiveBillingAccountPage />)

    expect(screen.getByText('Setting billing for')).toBeTruthy()
    expect(screen.getByText('Casey')).toBeTruthy()

    fireEvent.click(screen.getByText('Activate'))
    await flushAsync()

    expect(mockCreateCheckoutSession).toHaveBeenCalledWith({
      billingAccountId: 'ba_1',
      successUrl: 'https://account.spacewave.example/checkout/success',
      cancelUrl: 'https://account.spacewave.example/checkout/cancel',
    })
    expect(mockNavigateSession).toHaveBeenCalledWith({ path: 'setup' })
  })

  it('watches checkout status and redirects to setup after Stripe completes', async () => {
    mockCreateCheckoutSession.mockResolvedValue({
      checkoutUrl: 'https://checkout.stripe.com/test',
      status: 1,
    })

    const { rerender } = render(<NoActiveBillingAccountPage />)

    fireEvent.click(screen.getByText('Activate'))
    await flushAsync()

    expect(window.open).toHaveBeenCalledWith(
      'https://checkout.stripe.com/test',
      '_blank',
    )
    expect(
      screen.getByText(
        'Activating subscription, this page will update when confirmation arrives.',
      ),
    ).toBeTruthy()

    streamingValue = { status: 2 }
    rerender(<NoActiveBillingAccountPage />)
    await flushAsync()

    expect(mockNavigateSession).toHaveBeenCalledWith({ path: 'setup' })
  })

  it('assigns an already-active billing account instead of opening checkout', async () => {
    mockUsePromise.mockReturnValue({
      data: {
        accounts: [
          {
            id: 'ba_1',
            displayName: 'Billing Account',
            subscriptionStatus: 'active',
            lifecycleState: 'active',
            assignees: [],
          },
        ],
      },
      loading: false,
      error: null,
    })

    render(<NoActiveBillingAccountPage />)

    fireEvent.click(screen.getByText('Use this billing account'))
    await flushAsync()

    expect(mockAssignBillingAccount).toHaveBeenCalledWith(
      'ba_1',
      'account',
      'acct_1',
    )
    expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
    expect(mockNavigateSession).toHaveBeenCalledWith({ path: 'setup' })
  })

  it('redirects to setup when the target already has assigned active billing', () => {
    mockUsePromise.mockReturnValue({
      data: {
        accounts: [
          {
            id: 'ba_1',
            displayName: 'Billing Account',
            subscriptionStatus: 'trialing',
            lifecycleState: 'active',
            assignees: [{ ownerType: 'account', ownerId: 'acct_1' }],
          },
        ],
      },
      loading: false,
      error: null,
    })

    render(<NoActiveBillingAccountPage />)

    expect(screen.getByTestId('redirect').textContent).toBe('../../setup')
  })
})
