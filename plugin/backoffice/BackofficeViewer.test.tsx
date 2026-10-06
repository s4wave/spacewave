import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import BackofficeViewer from './BackofficeViewer.js'

const mockGetAdminJson = vi.hoisted(() => vi.fn())
const mockSession = vi.hoisted(() => ({
  spacewave: { getAdminJson: mockGetAdminJson },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => null },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockSession,
}))

// adminRoutes holds coordinator responses in the shape its admin routes write.
const adminRoutes: Record<string, unknown> = {
  'billing/summary': {
    mrr: 48,
    arr: 576,
    subscribers: {
      active: 6,
      past_due: 1,
      canceled: 2,
      sponsored: 1,
      total: 10,
    },
    meter_failed_accounts: 0,
    milestones: { breakeven: false, ramen_profitable: false, traction: false },
  },
  'accounts?limit=200&sort=last_active_at': {
    accounts: [
      {
        id: '01ACCOUNT',
        entityId: 'casey',
        email: 'casey@example.com',
        emailVerifiedAt: 1759700000000,
        subscriptionStatus: 'active',
        lifecycleState: 'active',
        lifecycleUpdatedAt: 1759700000000,
        deletedAt: 0,
        createdAt: 1759700000000,
        lastActiveAt: 1759710000000,
      },
    ],
    limit: 200,
    offset: 0,
  },
  'billing/accounts?limit=200': {
    accounts: [
      {
        id: '01PAYER',
        createdByAccountId: '01ACCOUNT',
        displayName: 'Casey',
        stripeCustomerId: 'cus_1',
        stripeSubscriptionId: 'sub_1',
        subscriptionStatus: 'active',
        lifecycleState: 'active',
        offerVersion: 'v1',
        sponsored: false,
        overageLimitCents: 0,
        residentBytes: 1048576,
        readOps: 3,
        writeOps: 4,
        meteredThroughAt: 0,
        accruedOverageMicrodollars: 0,
        reportedOverageMicrodollars: 0,
        failedMeterEvents: 0,
        pastDueSince: 0,
        updatedAt: 1759700000000,
      },
    ],
    limit: 200,
    offset: 0,
  },
  'accounts/01ACCOUNT/resources': {
    sharedObjects: [
      {
        id: '01SPACE',
        createdBy: '01ACCOUNT',
        displayName: 'Garden notes',
        objectType: 'space',
        accountPrivate: false,
        ownerType: 'account',
        ownerId: '01ACCOUNT',
        publicRead: false,
        createdAt: 1759700000000,
        updatedAt: 1759700000000,
      },
    ],
    blockStores: [
      {
        id: '01STORE',
        createdBy: '01ACCOUNT',
        ownerType: 'account',
        ownerId: '01ACCOUNT',
        createdAt: 1759700000000,
        dmcaNoticeId: '',
        dmcaBlockedAt: 0,
        liveBytes: 2048,
      },
    ],
  },
}

const props = {} as ObjectViewerComponentProps

describe('BackofficeViewer', () => {
  afterEach(() => {
    cleanup()
    mockGetAdminJson.mockReset()
  })

  it('shows an access message and no data to a non-admin account', async () => {
    mockGetAdminJson.mockRejectedValue(
      new Error('403 rbac_denied: Access denied'),
    )
    render(<BackofficeViewer {...props} />)

    expect(await screen.findByText('No access to the backoffice')).toBeDefined()
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('shows billing, accounts, payers and one account resources', async () => {
    mockGetAdminJson.mockImplementation((path: string) => {
      const body = adminRoutes[path]
      if (!body) return Promise.reject(new Error(`unexpected route ${path}`))
      return Promise.resolve(JSON.stringify(body))
    })
    render(<BackofficeViewer {...props} />)

    expect(await screen.findByText('casey@example.com')).toBeDefined()
    expect(screen.getByText('$48.00')).toBeDefined()
    expect(screen.getByText('1.0 MiB')).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: /casey@example.com/ }))
    expect(await screen.findByText('Garden notes')).toBeDefined()
    expect(screen.getByText('2.0 KiB')).toBeDefined()
  })
})
