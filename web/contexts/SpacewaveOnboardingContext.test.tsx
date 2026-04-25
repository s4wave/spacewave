import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  AccountLifecycleState,
  BillingStatus,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { SpacewaveOnboardingContext } from './SpacewaveOnboardingContext.js'

afterEach(() => {
  cleanup()
})

function ContextProbe() {
  const ctx = SpacewaveOnboardingContext.useContext()
  return (
    <div data-testid="has-active-billing">
      {ctx.hasActiveBilling ? 'true' : 'false'}
    </div>
  )
}

function renderContext(
  onboarding: Partial<WatchOnboardingStatusResponse> = {},
) {
  render(
    <SpacewaveOnboardingContext.Provider
      onboarding={onboarding as WatchOnboardingStatusResponse}
    >
      <ContextProbe />
    </SpacewaveOnboardingContext.Provider>,
  )
  return screen.getByTestId('has-active-billing').textContent
}

describe('SpacewaveOnboardingContext', () => {
  it.each([
    {
      name: 'TRIALING counts as active billing',
      onboarding: {
        subscriptionStatus: BillingStatus.BillingStatus_TRIALING,
      },
      expected: 'true',
    },
    {
      name: 'ACTIVE counts as active billing',
      onboarding: {
        subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
      },
      expected: 'true',
    },
    {
      name: 'PAST_DUE does not count as active billing',
      onboarding: {
        subscriptionStatus: BillingStatus.BillingStatus_PAST_DUE,
      },
      expected: 'false',
    },
    {
      name: 'CANCELED does not count as active billing',
      onboarding: {
        subscriptionStatus: BillingStatus.BillingStatus_CANCELED,
      },
      expected: 'false',
    },
    {
      name: 'lifecycleState ACTIVE without an active subscription does not count',
      onboarding: {
        lifecycleState: AccountLifecycleState.AccountLifecycleState_ACTIVE,
        subscriptionStatus: BillingStatus.BillingStatus_NONE,
      },
      expected: 'false',
    },
    {
      name: 'ACTIVE_WITH_CANCEL_AT_PERIOD_END plus active subscription counts',
      onboarding: {
        lifecycleState:
          AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END,
        subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
      },
      expected: 'true',
    },
  ])('$name', ({ onboarding, expected }) => {
    expect(
      renderContext(onboarding as Partial<WatchOnboardingStatusResponse>),
    ).toBe(expected)
  })
})
