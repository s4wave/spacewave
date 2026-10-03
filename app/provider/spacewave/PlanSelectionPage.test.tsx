import { CLOUD_OFFER } from './pricing.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, cleanup, fireEvent, screen, act } from '@testing-library/react'
import { PlanSelectionPage } from './PlanSelectionPage.js'

const mockPath = vi.hoisted(() => ({ value: '/u/0/plan' }))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: vi.fn(() => vi.fn()),
  usePath: () => mockPath.value,
  useParams: vi.fn(() => ({ sessionIndex: '0' })),
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

const mockSessionResource = vi.hoisted(() => ({
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
  useSessionIndex: () => 0,
}))

const mockUseSessionMetadata = vi.hoisted(() =>
  vi.fn((): { providerId: string } | null => null),
)

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: vi.fn(),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: vi.fn(() => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  })),
}))

vi.mock('@aptre/bldr', () => ({
  isDesktop: false,
}))

const mockUseCloudProviderConfig = vi.hoisted(() =>
  vi.fn<() => { accountBaseUrl?: string; publicBaseUrl?: string } | null>(
    () => ({
      accountBaseUrl: 'https://account.spacewave.example',
      publicBaseUrl: 'https://spacewave.example',
    }),
  ),
)

vi.mock('./useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: mockUseCloudProviderConfig,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingStatus: { BillingStatus_ACTIVE: 2 },
  CheckoutStatus: {
    CheckoutStatus_UNKNOWN: 0,
    CheckoutStatus_PENDING: 1,
    CheckoutStatus_COMPLETED: 2,
    CheckoutStatus_EXPIRED: 3,
    CheckoutStatus_CANCELED: 4,
  },
}))

import { useNavigate } from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

const mockUseNavigate = useNavigate as ReturnType<typeof vi.fn>
const mockUseResourceValue = useResourceValue as ReturnType<typeof vi.fn>

// flushAsync flushes pending microtasks from fire-and-forget async handlers.
async function flushAsync() {
  await act(async () => {})
  await act(async () => {})
}

async function acceptMonthlyOffer() {
  fireEvent.click(
    screen.getByRole('button', { name: 'Subscribe for $8/month' }),
  )
  await flushAsync()
}

describe('PlanSelectionPage', () => {
  const mockNavigate = vi.fn()
  const mockCreateCheckoutSession = vi.fn()
  const mockCancelCheckoutSession = vi.fn()
  const mockSession = {
    spacewave: {
      createCheckoutSession: mockCreateCheckoutSession,
      watchCheckoutStatus: vi.fn(),
      cancelCheckoutSession: mockCancelCheckoutSession,
    },
  }

  beforeEach(() => {
    cleanup()
    mockNavigate.mockClear()
    mockCreateCheckoutSession.mockClear()
    mockCancelCheckoutSession.mockClear()

    mockUseNavigate.mockReturnValue(mockNavigate)
    mockUseResourceValue.mockReturnValue(mockSession)
    mockUseSessionMetadata.mockReset()
    mockUseSessionMetadata.mockReturnValue(null)
    mockUseCloudProviderConfig.mockReset()
    mockUseCloudProviderConfig.mockReturnValue({
      accountBaseUrl: 'https://account.spacewave.example',
      publicBaseUrl: 'https://spacewave.example',
    })

    vi.spyOn(window, 'open').mockReturnValue({} as Window)

    window.location.hash = '#/u/0/plan'
    mockPath.value = '/u/0/plan'
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('Rendering', () => {
    it('renders the welcome header', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Welcome to Spacewave')).toBeDefined()
    })

    it('renders the animated logo', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByTestId('animated-logo')).toBeDefined()
    })

    it('renders the Cloud plan card with monthly price', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Cloud')).toBeDefined()
      expect(screen.getByText('$8')).toBeDefined()
      expect(screen.getByText('/ month')).toBeDefined()
    })

    it('renders cloud features', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Cloud sync and backup')).toBeDefined()
      expect(screen.getByText('Shared Spaces with collaborators')).toBeDefined()
      expect(screen.getByText('100 GiB cloud storage included')).toBeDefined()
      expect(
        screen.getByText(
          'Extra storage at $0.02 per GiB-month, reads and writes not billed',
        ),
      ).toBeDefined()
      expect(
        screen.getByText('Always-on sync across all devices'),
      ).toBeDefined()
      expect(screen.getByText('End-to-end encrypted')).toBeDefined()
    })

    it('renders the Start with Cloud button', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Start with Cloud')).toBeDefined()
    })

    it('renders the free local storage option with correct copy', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Continue with local storage')).toBeDefined()
      expect(screen.getByText('Store on your own devices')).toBeDefined()
      expect(screen.getByText('Free and open-source')).toBeDefined()
      expect(screen.getByText('No cloud account required')).toBeDefined()
    })

    it('renders shared features', () => {
      render(<PlanSelectionPage />)
      expect(screen.getByText('Both options include')).toBeDefined()
      expect(screen.getByText('The full local-first app')).toBeDefined()
      expect(
        screen.getByText('Full plugin SDK and developer tools'),
      ).toBeDefined()
      expect(
        screen.getByText('Peer-to-peer sync between devices'),
      ).toBeDefined()
      expect(screen.getByText('Open-source, self-hostable')).toBeDefined()
    })
  })

  describe('Cloud Checkout', () => {
    it('disables Start with Cloud button when session is null', () => {
      mockUseResourceValue.mockReturnValue(null)
      render(<PlanSelectionPage />)
      const button = screen.getByText('Start with Cloud').closest('button')
      expect(button?.hasAttribute('disabled')).toBe(true)
    })

    it('navigates to upgrade page when Start with Cloud is clicked', () => {
      render(<PlanSelectionPage />)
      const button = screen.getByText('Start with Cloud').closest('button')

      act(() => {
        fireEvent.click(button!)
      })

      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/0/plan/upgrade',
      })
    })

    it('uses the panel route instead of the shell hash when split', () => {
      window.location.hash = '#/g/encoded-shell-layout'
      mockPath.value = '/u/0/plan'

      render(<PlanSelectionPage />)
      const button = screen.getByText('Start with Cloud').closest('button')

      act(() => {
        fireEvent.click(button!)
      })

      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/0/plan/upgrade',
      })
    })

    it('waits for affirmative monthly consent before creating checkout', async () => {
      mockCreateCheckoutSession.mockResolvedValue({
        checkoutUrl: 'https://checkout.stripe.com/test',
        status: 1, // PENDING
      })

      render(<PlanSelectionPage startCloud />)
      await flushAsync()
      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
      await acceptMonthlyOffer()

      expect(mockCreateCheckoutSession).toHaveBeenCalledWith({
        successUrl: 'https://account.spacewave.example/checkout/success',
        cancelUrl: 'https://account.spacewave.example/checkout/cancel',
        consent: {
          offerVersion: CLOUD_OFFER.version,
          policyVersion: CLOUD_OFFER.policyVersion,
          renewalAccepted: true,
          overageAccepted: true,
          overageLimitCents: CLOUD_OFFER.defaultOverageLimitCents,
        },
      })
    })

    it('lets the customer turn extra usage off with no agreement checkbox or additional confirmation', async () => {
      mockCreateCheckoutSession.mockResolvedValue({
        checkoutUrl: 'https://checkout.stripe.com/test',
        status: 1,
      })
      render(<PlanSelectionPage startCloud />)
      expect(screen.queryByRole('checkbox')).toBeNull()
      const limit = screen.getByRole('combobox') as HTMLSelectElement
      expect(limit.value).toBe(String(CLOUD_OFFER.defaultOverageLimitCents))
      fireEvent.change(limit, { target: { value: '0' } })
      await acceptMonthlyOffer()
      expect(mockCreateCheckoutSession).toHaveBeenCalledWith(
        expect.objectContaining({
          consent: expect.objectContaining({
            renewalAccepted: true,
            overageAccepted: false,
            overageLimitCents: 0,
          }),
        }),
      )
    })

    it('does not navigate to setup while checkout is still pending', async () => {
      mockCreateCheckoutSession.mockResolvedValue({
        checkoutUrl: 'https://checkout.stripe.com/test',
        status: 1, // PENDING
      })

      render(<PlanSelectionPage startCloud />)
      await flushAsync()
      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
      await acceptMonthlyOffer()

      expect(mockNavigate).not.toHaveBeenCalled()
    })

    it('does not start checkout before cloud provider config loads', async () => {
      mockUseCloudProviderConfig.mockReturnValue(null)

      render(<PlanSelectionPage startCloud />)
      await flushAsync()

      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
    })

    it('navigates to setup when checkout status is already completed', async () => {
      mockCreateCheckoutSession.mockResolvedValue({
        checkoutUrl: '',
        status: 2, // COMPLETED
      })

      render(<PlanSelectionPage startCloud />)
      await flushAsync()
      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
      await acceptMonthlyOffer()

      expect(mockNavigate).toHaveBeenCalled()
    })

    it('displays error when createCheckoutSession fails', async () => {
      mockCreateCheckoutSession.mockRejectedValue(new Error('Stripe error'))

      render(<PlanSelectionPage startCloud />)
      await flushAsync()
      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
      await acceptMonthlyOffer()

      expect(screen.getByText('Stripe error')).toBeDefined()
    })

    it('displays generic error message for non-Error throws', async () => {
      mockCreateCheckoutSession.mockRejectedValue('unknown failure')

      render(<PlanSelectionPage startCloud />)
      await flushAsync()
      expect(mockCreateCheckoutSession).not.toHaveBeenCalled()
      await acceptMonthlyOffer()

      expect(screen.getByText('Failed to create checkout')).toBeDefined()
    })
  })

  describe('Free Local Session', () => {
    it('navigates to free local setup on button click', () => {
      render(<PlanSelectionPage />)
      const button = screen
        .getByText('Continue with local storage')
        .closest('button')

      act(() => {
        fireEvent.click(button!)
      })

      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/0/plan/free',
      })
    })

    it('navigates directly to setup for local sessions', () => {
      mockUseSessionMetadata.mockReturnValue({ providerId: 'local' })

      render(<PlanSelectionPage />)
      const button = screen
        .getByText('Continue with local storage')
        .closest('button')

      act(() => {
        fireEvent.click(button!)
      })

      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/u/0/setup',
      })
    })

    it('disables free local button when session is null', () => {
      mockUseResourceValue.mockReturnValue(null)
      render(<PlanSelectionPage />)
      const button = screen
        .getByText('Continue with local storage')
        .closest('button')
      expect(button?.hasAttribute('disabled')).toBe(true)
    })
  })

  describe('Subscription Watch', () => {
    it('renders activating subscription view when checkoutResult is success', () => {
      render(<PlanSelectionPage checkoutResult="success" />)

      expect(screen.getByText('Activating subscription…')).toBeDefined()
    })
  })

  describe('Session Index', () => {
    it('renders Start with Cloud button with session available', () => {
      render(<PlanSelectionPage />)

      const button = screen.getByText('Start with Cloud').closest('button')
      expect(button).toBeDefined()
    })
  })
})
