import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import {
  AccountLifecycleState,
  BillingStatus,
  SelfEnrollmentGateState,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'

import { SpacewaveRootRouter } from './SpacewaveRootRouter.js'

const mockUseContextSafe = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockBillingStateProvider = vi.hoisted(() => vi.fn())
const mockToastInfo = vi.hoisted(() => vi.fn())
const mockSelfEnrollmentSkip = vi.hoisted<{
  value: { skippedKey: string; skippedAt: number } | null
}>(() => ({ value: null }))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: mockUseContextSafe,
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: ({ to }: { to: string }) => <div data-testid="redirect">{to}</div>,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    info: mockToastInfo,
  },
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateAtom: () => [mockSelfEnrollmentSkip.value, vi.fn()],
}))

vi.mock('@s4wave/app/billing/BillingStateProvider.js', () => ({
  BillingStateProvider: (props: { children?: ReactNode }) => {
    mockBillingStateProvider()
    return <div data-testid="billing-provider">{props.children}</div>
  },
}))

vi.mock('@s4wave/app/session/SessionDashboardContainer.js', () => ({
  SessionDashboardContainer: () => <div data-testid="dashboard" />,
}))

vi.mock('@s4wave/web/ui/loading/LoadingCard.js', () => ({
  LoadingCard: ({ view }: { view: { detail?: string } }) => (
    <div data-testid="loading-card">{view.detail ?? ''}</div>
  ),
}))

vi.mock('./SessionSelfEnrollmentInterstitial.js', () => ({
  SessionSelfEnrollmentInterstitial: () => (
    <div data-testid="self-enrollment-interstitial" />
  ),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockSelfEnrollmentSkip.value = null
})

function buildContext(
  overrides: {
    onboarding?: {
      accountStatus?: ProviderAccountStatus
      hasLinkedLocal?: boolean
      linkedLocalSessionIndex?: number
      subscriptionStatus?: BillingStatus
      lifecycleState?: AccountLifecycleState
      managedBaCount?: number
      managedNoSubscriptionBaCount?: number
      billingSummaryLoaded?: boolean
      selfEnrollmentGateState?: SelfEnrollmentGateState
      sessionSelfEnrollmentGenerationKey?: string
      sessionSelfEnrollmentCount?: number
    }
    isLapsed?: boolean
    hasActiveBilling?: boolean
    emailVerified?: boolean
  } = {},
) {
  const { onboarding: onboardingOverrides, ...ctxOverrides } = overrides
  const onboarding = {
    accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
    hasLinkedLocal: false,
    subscriptionStatus: BillingStatus.BillingStatus_NONE,
    managedBaCount: 0,
    managedNoSubscriptionBaCount: 0,
    billingSummaryLoaded: true,
    selfEnrollmentGateState: SelfEnrollmentGateState.READY,
    ...onboardingOverrides,
  }
  return {
    onboarding,
    isLapsed: false,
    hasActiveBilling: false,
    emailVerified: false,
    ...ctxOverrides,
  }
}

describe('SpacewaveRootRouter', () => {
  it('wraps the dashboard in billing state for active sessions', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('billing-provider')).toBeTruthy()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
    expect(mockBillingStateProvider).toHaveBeenCalledTimes(1)
  })

  it('renders the loading card while the cloud account snapshot is still loading', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          accountStatus: ProviderAccountStatus.ProviderAccountStatus_NONE,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.getByTestId('loading-card').textContent).toBe(
      'Fetching account status.',
    )
    expect(screen.queryByTestId('redirect')).toBeNull()
    expect(screen.queryByTestId('dashboard')).toBeNull()
  })

  it('renders the loading card while account status is pending', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          accountStatus: ProviderAccountStatus.ProviderAccountStatus_PENDING,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('renders the loading card while the billing summary is still loading for inactive accounts', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          billingSummaryLoaded: false,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.getByTestId('loading-card').textContent).toBe(
      'Checking subscription status.',
    )
    expect(screen.queryByTestId('redirect')).toBeNull()
    expect(screen.queryByTestId('dashboard')).toBeNull()
  })

  it('routes inactive sessions with zero managed billing accounts to /plan', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          managedBaCount: 0,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan')
  })

  it('renders the dashboard when subscription is active', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.queryByTestId('redirect')).toBeNull()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
  })

  it('routes dormant accounts to the upgrade flow', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          accountStatus: ProviderAccountStatus.ProviderAccountStatus_DORMANT,
        },
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan/upgrade')
  })

  it('routes dormant accounts with a linked local session to the upgrade flow', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          accountStatus: ProviderAccountStatus.ProviderAccountStatus_DORMANT,
          hasLinkedLocal: true,
          linkedLocalSessionIndex: 2,
        },
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan/upgrade')
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('redirects linked-local inactive sessions into the local shell', async () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          hasLinkedLocal: true,
          linkedLocalSessionIndex: 2,
        },
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.getByTestId('loading-card').textContent).toBe(
      'Switching to your local session.',
    )
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/2' }),
    )
    expect(mockToastInfo).toHaveBeenCalledWith(
      'No subscription, using local session.',
    )
  })

  it('renders the dashboard in read-only mode for lapsed sessions', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_CANCELED,
        },
        isLapsed: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('billing-provider')).toBeTruthy()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('routes active but unverified sessions to email verification', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
        },
        hasActiveBilling: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('verify-email')
  })

  it('keeps email verification ahead of self-enrollment summary loading', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.CHECKING,
        },
        hasActiveBilling: true,
        emailVerified: false,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('verify-email')
  })

  it('renders the loading card while the self-enrollment summary is still loading', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.CHECKING,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.getByTestId('loading-card').textContent).toBe(
      'Checking connected spaces.',
    )
    expect(screen.queryByTestId('self-enrollment-interstitial')).toBeNull()
    expect(screen.queryByTestId('dashboard')).toBeNull()
  })

  it('routes active verified sessions needing self-enrollment to the interstitial', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.ACTION_REQUIRED,
          sessionSelfEnrollmentGenerationKey: 'gen-1',
          sessionSelfEnrollmentCount: 2,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('self-enrollment-interstitial')).toBeTruthy()
    expect(screen.queryByTestId('dashboard')).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
  })

  it('keeps the loading card while backend auto-rejoin is running', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.AUTO_CONNECTING,
          sessionSelfEnrollmentGenerationKey: 'gen-1',
          sessionSelfEnrollmentCount: 1,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('session-loading')).toBeTruthy()
    expect(screen.getByTestId('loading-card').textContent).toBe(
      'Connecting spaces.',
    )
    expect(screen.queryByTestId('self-enrollment-interstitial')).toBeNull()
    expect(screen.queryByTestId('dashboard')).toBeNull()
  })

  it('routes active verified sessions with a matching skip atom to the dashboard', () => {
    mockSelfEnrollmentSkip.value = {
      skippedKey: 'gen-1',
      skippedAt: 1,
    }
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.ACTION_REQUIRED,
          sessionSelfEnrollmentGenerationKey: 'gen-1',
          sessionSelfEnrollmentCount: 2,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.queryByTestId('self-enrollment-interstitial')).toBeNull()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
  })

  it('shows the interstitial when the skip atom is for an old generation', () => {
    mockSelfEnrollmentSkip.value = {
      skippedKey: 'old-gen',
      skippedAt: 1,
    }
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.ACTION_REQUIRED,
          sessionSelfEnrollmentGenerationKey: 'gen-1',
          sessionSelfEnrollmentCount: 2,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('self-enrollment-interstitial')).toBeTruthy()
    expect(screen.queryByTestId('dashboard')).toBeNull()
  })

  it('keeps email verification ahead of self-enrollment', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
          selfEnrollmentGateState: SelfEnrollmentGateState.ACTION_REQUIRED,
          sessionSelfEnrollmentGenerationKey: 'gen-1',
          sessionSelfEnrollmentCount: 2,
        },
        hasActiveBilling: true,
        emailVerified: false,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('verify-email')
    expect(screen.queryByTestId('self-enrollment-interstitial')).toBeNull()
  })

  it('treats trialing billing as active for routing', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_TRIALING,
        },
        hasActiveBilling: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('billing-provider')).toBeTruthy()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('routes inactive sessions with only no-subscription managed billing accounts to /plan', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          managedBaCount: 2,
          managedNoSubscriptionBaCount: 2,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan')
  })

  it('routes inactive sessions with reactivatable managed billing accounts to /plan/no-active', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          managedBaCount: 2,
          managedNoSubscriptionBaCount: 1,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan/no-active')
  })

  it('routes past-due billing by managed billing account count', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          subscriptionStatus: BillingStatus.BillingStatus_PAST_DUE,
          managedBaCount: 1,
          managedNoSubscriptionBaCount: 0,
        },
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('plan/no-active')
    expect(screen.queryByTestId('billing-provider')).toBeNull()
  })

  it('keeps pending-delete accounts in the cloud shell even with a linked local session', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          hasLinkedLocal: true,
          linkedLocalSessionIndex: 2,
          subscriptionStatus: BillingStatus.BillingStatus_CANCELED,
          lifecycleState:
            AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY,
        },
        isLapsed: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('billing-provider')).toBeTruthy()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('keeps deleted accounts in the cloud shell even with a linked local session', () => {
    mockUseContextSafe.mockReturnValue(
      buildContext({
        onboarding: {
          hasLinkedLocal: true,
          linkedLocalSessionIndex: 2,
          subscriptionStatus: BillingStatus.BillingStatus_CANCELED,
          lifecycleState:
            AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE,
        },
        isLapsed: true,
        emailVerified: true,
      }),
    )

    render(<SpacewaveRootRouter />)

    expect(screen.getByTestId('billing-provider')).toBeTruthy()
    expect(screen.getByTestId('dashboard')).toBeTruthy()
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
