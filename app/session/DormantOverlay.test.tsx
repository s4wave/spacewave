import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DormantOverlay } from './DormantOverlay.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockListManagedBillingAccounts = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    }),
  },
  useSessionNavigate: () => mockNavigateSession,
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

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => null,
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...parts: Array<string | false | null | undefined>) =>
    parts.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingStatus: {
    BillingStatus_CANCELED: 5,
  },
  AccountLifecycleState: {
    AccountLifecycleState_CANCELED_GRACE_READONLY: 3,
    AccountLifecycleState_LAPSED_READONLY: 4,
  },
}))

describe('DormantOverlay', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockNavigateSession.mockReset()
    mockListManagedBillingAccounts.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('routes to the targeted billing account reactivation flow', async () => {
    mockListManagedBillingAccounts.mockResolvedValue({
      accounts: [
        {
          id: 'ba_personal',
          subscriptionStatus: 5,
          assignees: [{ ownerType: 'account', ownerId: 'acct_personal' }],
        },
      ],
    })

    render(
      <DormantOverlay
        metadata={{ displayName: 'Cloud Session', cloudEntityId: 's1' }}
      />,
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() =>
      expect(mockNavigateSession).toHaveBeenCalledWith({
        path: 'billing/ba_personal?reactivate=1',
      }),
    )
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

    render(
      <DormantOverlay
        metadata={{ displayName: 'Cloud Session', cloudEntityId: 's1' }}
      />,
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() =>
      expect(mockNavigateSession).toHaveBeenCalledWith({
        path: 'plan/no-active',
      }),
    )
  })
})
