import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'

import { UpgradeRouter } from './UpgradeRouter.js'

const mockSessionResource = vi.hoisted(() => ({ value: {} }))
const mockUseSessionInfo = vi.hoisted(() =>
  vi.fn(() => ({ providerId: 'spacewave', isCloud: true })),
)
const mockUseContextSafe = vi.hoisted(() => vi.fn())
const mockCloudConfirmationPage = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockPath = vi.hoisted(() => ({ value: '/u/0/plan/upgrade' }))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockSessionResource.value,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  usePath: () => mockPath.value,
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: ({ to }: { to: string }) => <div data-testid="redirect">{to}</div>,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: mockUseContextSafe,
  },
}))

vi.mock('./CloudConfirmationPage.js', () => ({
  CloudConfirmationPage: (props: unknown) => {
    mockCloudConfirmationPage(props)
    return <div data-testid="cloud-confirmation" />
  },
}))

vi.mock('./checkout-url.js', () => ({
  getCheckoutResultBaseUrl: () => 'https://app.example',
}))

vi.mock('./useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => ({}),
}))

function buildCtx(
  overrides: {
    accountStatus?: ProviderAccountStatus
    hasSubscription?: boolean
    hasLinkedLocal?: boolean
    linkedLocalHasContent?: boolean
  } = {},
) {
  return {
    onboarding: {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      subscriptionStatus: BillingStatus.BillingStatus_ACTIVE,
      hasSubscription: true,
      hasLinkedLocal: false,
      linkedLocalHasContent: false,
      ...overrides,
    },
  }
}

describe('UpgradeRouter', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    mockSessionResource.value = {}
    mockUseSessionInfo.mockReturnValue({
      providerId: 'spacewave',
      isCloud: true,
    })
    mockPath.value = '/u/0/plan/upgrade'
  })

  it('renders nothing until the cloud account snapshot is loaded', () => {
    mockUseContextSafe.mockReturnValue({ onboarding: null })

    const { container } = render(<UpgradeRouter />)

    expect(container.firstChild).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
    expect(mockCloudConfirmationPage).not.toHaveBeenCalled()
  })

  it('renders nothing while the cloud account status is still a placeholder', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_PENDING,
      }),
    )

    const { container } = render(<UpgradeRouter />)

    expect(container.firstChild).toBeNull()
    expect(screen.queryByTestId('redirect')).toBeNull()
    expect(mockCloudConfirmationPage).not.toHaveBeenCalled()
  })

  it('sends subscribed callers with non-empty linked local content to migration', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        hasLinkedLocal: true,
        linkedLocalHasContent: true,
      }),
    )

    render(<UpgradeRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('../migrate')
    expect(mockCloudConfirmationPage).not.toHaveBeenCalled()
  })

  it('sends subscribed callers without linked local content back to the dashboard', () => {
    mockUseContextSafe.mockReturnValue(
      buildCtx({
        hasLinkedLocal: true,
        linkedLocalHasContent: false,
      }),
    )

    render(<UpgradeRouter />)

    expect(screen.getByTestId('redirect').textContent).toBe('../../')
    expect(mockCloudConfirmationPage).not.toHaveBeenCalled()
  })
})
