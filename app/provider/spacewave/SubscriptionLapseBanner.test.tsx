import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SubscriptionLapseBanner } from './SubscriptionLapseBanner.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockListManagedBillingAccounts = vi.hoisted(() => vi.fn())
const mockSetOpenMenu = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    }),
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => ({
    spacewave: {
      listManagedBillingAccounts: mockListManagedBillingAccounts,
    },
  }),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({
    accountId: 'acct_personal',
  }),
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: () => ({
      onboarding: {
        lifecycleState: 3,
      },
      isReadOnlyGrace: false,
    }),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParentPaths: () => ['/u/1'],
  usePath: () => '/u/1',
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mockSetOpenMenu,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', async () => {
  const actual = await vi.importActual<
    typeof import('@s4wave/sdk/provider/spacewave/spacewave.pb.js')
  >('@s4wave/sdk/provider/spacewave/spacewave.pb.js')
  return {
    ...actual,
    AccountLifecycleState: {
      ...actual.AccountLifecycleState,
      AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END: 2,
      AccountLifecycleState_CANCELED_GRACE_READONLY: 3,
      AccountLifecycleState_LAPSED_READONLY: 4,
      AccountLifecycleState_DELETED_PENDING_PURGE: 5,
      AccountLifecycleState_DELETED: 6,
    },
  }
})

describe('SubscriptionLapseBanner', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockListManagedBillingAccounts.mockReset()
    mockSetOpenMenu.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('routes the whole banner to the targeted billing account reactivation flow', async () => {
    mockListManagedBillingAccounts.mockResolvedValue({
      accounts: [
        {
          id: 'ba_personal',
          subscriptionStatus: 5,
          assignees: [{ ownerType: 'account', ownerId: 'acct_personal' }],
        },
      ],
    })

    render(<SubscriptionLapseBanner />)
    fireEvent.click(screen.getByRole('button', { name: /resubscribe/i }))

    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/1/billing/ba_personal?reactivate=1',
      }),
    )
    expect(mockSetOpenMenu).toHaveBeenCalledWith('')
  })

  it('routes to /plan/no-active when no personal canceled BA exists', async () => {
    mockListManagedBillingAccounts.mockResolvedValue({
      accounts: [
        {
          id: 'ba_org',
          subscriptionStatus: 5,
          assignees: [{ ownerType: 'organization', ownerId: 'org_1' }],
        },
      ],
    })

    render(<SubscriptionLapseBanner />)
    fireEvent.click(screen.getByRole('button', { name: /resubscribe/i }))

    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/1/plan/no-active',
      }),
    )
  })
})
