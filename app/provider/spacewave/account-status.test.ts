import { describe, expect, it } from 'vitest'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import {
  BillingStatus,
  type WatchOnboardingStatusResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import {
  hasReactivatableManagedBilling,
  isAccountStatusLoaded,
  isOnboardingReady,
} from './account-status.js'

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

  it.each([
    {
      name: 'active',
      onboarding: {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
        subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
        managedBaCount: 0,
        managedNoSubscriptionBaCount: 0,
        billingSummaryLoaded: true,
      },
      ready: true,
      reactivatableManagedBilling: false,
    },
    {
      name: 'inactive',
      onboarding: {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
        subscriptionStatus: BillingStatus.BillingStatus_NONE,
        managedBaCount: 0,
        managedNoSubscriptionBaCount: 0,
        billingSummaryLoaded: true,
      },
      ready: true,
      reactivatableManagedBilling: false,
    },
    {
      name: 'no-active',
      onboarding: {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
        subscriptionStatus: BillingStatus.BillingStatus_CANCELED,
        managedBaCount: 1,
        managedNoSubscriptionBaCount: 0,
        billingSummaryLoaded: true,
      },
      ready: true,
      reactivatableManagedBilling: true,
    },
    {
      name: 'billing-summary-not-loaded',
      onboarding: {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
        subscriptionStatus: BillingStatus.BillingStatus_UNKNOWN,
        managedBaCount: 0,
        managedNoSubscriptionBaCount: 0,
        billingSummaryLoaded: false,
      },
      ready: false,
      reactivatableManagedBilling: false,
    },
  ])(
    'characterizes billing route readiness for $name',
    ({ onboarding, ready, reactivatableManagedBilling }) => {
      expect(
        isOnboardingReady(onboarding as WatchOnboardingStatusResponse),
      ).toBe(ready)
      if (ready) {
        expect(
          hasReactivatableManagedBilling(
            onboarding as WatchOnboardingStatusResponse,
          ),
        ).toBe(reactivatableManagedBilling)
      }
    },
  )

  it.each([
    {
      name: 'zero managed billing accounts',
      onboarding: {
        managedBaCount: 0,
        managedNoSubscriptionBaCount: 0,
      },
      expected: false,
    },
    {
      name: 'every managed billing account has no subscription',
      onboarding: {
        managedBaCount: 2,
        managedNoSubscriptionBaCount: 2,
      },
      expected: false,
    },
    {
      name: 'at least one managed billing account is reactivatable',
      onboarding: {
        managedBaCount: 2,
        managedNoSubscriptionBaCount: 1,
      },
      expected: true,
    },
  ])('detects $name', ({ onboarding, expected }) => {
    expect(
      hasReactivatableManagedBilling(
        onboarding as WatchOnboardingStatusResponse,
      ),
    ).toBe(expected)
  })
})
