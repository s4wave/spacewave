import { describe, expect, it } from 'vitest'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { isAccountStatusLoaded, isOnboardingReady } from './account-status.js'

describe('account status route predicates', () => {
  it.each([
    {
      name: 'undefined',
      status: undefined,
      expected: false,
    },
    {
      name: 'NONE',
      status: ProviderAccountStatus.ProviderAccountStatus_NONE,
      expected: false,
    },
    {
      name: 'PENDING',
      status: ProviderAccountStatus.ProviderAccountStatus_PENDING,
      expected: false,
    },
    {
      name: 'READY',
      status: ProviderAccountStatus.ProviderAccountStatus_READY,
      expected: true,
    },
    {
      name: 'DORMANT',
      status: ProviderAccountStatus.ProviderAccountStatus_DORMANT,
      expected: true,
    },
    {
      name: 'UNAUTHENTICATED',
      status: ProviderAccountStatus.ProviderAccountStatus_UNAUTHENTICATED,
      expected: true,
    },
    {
      name: 'DELETED',
      status: ProviderAccountStatus.ProviderAccountStatus_DELETED,
      expected: true,
    },
    {
      name: 'FAILED',
      status: ProviderAccountStatus.ProviderAccountStatus_FAILED,
      expected: true,
    },
  ])('treats $name as loaded=$expected', ({ status, expected }) => {
    expect(isAccountStatusLoaded(status)).toBe(expected)
  })

  it('holds route readiness until the account and billing summary are loaded', () => {
    expect(isOnboardingReady(null)).toBe(false)
    expect(
      isOnboardingReady({
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_PENDING,
        billingSummaryLoaded: true,
      } as WatchOnboardingStatusResponse),
    ).toBe(false)
    expect(
      isOnboardingReady({
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      } as WatchOnboardingStatusResponse),
    ).toBe(false)
    expect(
      isOnboardingReady({
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
        billingSummaryLoaded: true,
      } as WatchOnboardingStatusResponse),
    ).toBe(true)
  })
})
