import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { PlanPageRouter } from './PlanPageRouter.js'

const mockUseSessionIndex = vi.hoisted(() => vi.fn(() => 3))
const mockUseContextSafe = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: mockUseSessionIndex,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: mockUseContextSafe,
  },
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: ({ to }: { to: string }) => <div data-testid="redirect">{to}</div>,
}))

vi.mock('./PlanSelectionPage.js', () => ({
  PlanSelectionPage: () => <div data-testid="plan-selection-page" />,
}))

type OnboardingOverrides = {
  accountStatus?: ProviderAccountStatus
  subscriptionStatus?: BillingStatus
  hasLinkedCloud?: boolean
  linkedCloudSessionIndex?: number
  managedBaCount?: number
  managedNoSubscriptionBaCount?: number
  billingSummaryLoaded?: boolean
}

function buildCtx(
  overrides: {
    onboarding?: OnboardingOverrides
    hasActiveBilling?: boolean
  } = {},
) {
  const { onboarding: onboardingOverrides, ...ctxOverrides } = overrides
  return {
    onboarding: {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      subscriptionStatus: BillingStatus.BillingStatus_NONE,
      managedBaCount: 0,
      managedNoSubscriptionBaCount: 0,
      billingSummaryLoaded: true,
      ...onboardingOverrides,
    },
    hasActiveBilling: false,
    ...ctxOverrides,
  }
}

describe('PlanPageRouter', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the plan selection page for local sessions without spacewave context', () => {
    mockUseContextSafe.mockReturnValue(null)

    render(<PlanPageRouter />)

    expect(screen.getByTestId('plan-selection-page')).toBeDefined()
  })

  it('renders nothing while the cloud account snapshot has not loaded', () => {
    mockUseContextSafe.mockReturnValue({
      onboarding: null,
      hasActiveBilling: false,
    })

    const { container } = render(<PlanPageRouter />)

    expect(container.firstChild).toBeNull()
    expect(screen.queryByTestId('plan-selection-page')).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('renders nothing while the cloud account status is still a placeholder', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          accountStatus: ProviderAccountStatus.ProviderAccountStatus_NONE,
        },
      }),
    )

    const { container } = render(<PlanPageRouter />)

    expect(container.firstChild).toBeNull()
    expect(screen.queryByTestId('plan-selection-page')).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('renders nothing while the billing summary is still loading', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          billingSummaryLoaded: false,
        },
      }),
    )

    const { container } = render(<PlanPageRouter />)

    expect(container.firstChild).toBeNull()
    expect(screen.queryByTestId('plan-selection-page')).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('renders the plan selection page once the account snapshot is loaded', () => {
    mockUseContextSafe.mockReturnValue(buildCtx())

    render(<PlanPageRouter />)

    expect(screen.getByTestId('plan-selection-page')).toBeDefined()
  })

  it('redirects off /plan when subscription status is active', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
        },
        hasActiveBilling: true,
      }),
    )

    render(<PlanPageRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('../')
  })

  it('redirects off /plan when subscription status is trialing', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_TRIALING,
        },
        hasActiveBilling: true,
      }),
    )

    render(<PlanPageRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('../')
  })

  it('redirects to the linked cloud plan page for another session', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          hasLinkedCloud: true,
          linkedCloudSessionIndex: 7,
        },
      }),
    )

    render(<PlanPageRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('/u/7/plan')
  })

  it('redirects to /plan/no-active when managed BAs include a reactivatable one', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          managedBaCount: 2,
          managedNoSubscriptionBaCount: 1,
        },
      }),
    )

    render(<PlanPageRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('no-active')
  })

  it('renders the plan selection page when every managed BA is in the NONE status', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        onboarding: {
          managedBaCount: 2,
          managedNoSubscriptionBaCount: 2,
        },
      }),
    )

    render(<PlanPageRouter />)

    expect(screen.getByTestId('plan-selection-page')).toBeDefined()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })
})
