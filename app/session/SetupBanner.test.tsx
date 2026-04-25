import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SetupBanner } from './SetupBanner.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockDismiss = vi.hoisted(() => vi.fn())
const mockUseLocalSessionOnboardingContext = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: vi.fn(() => mockNavigate),
  useParentPaths: vi.fn(() => ['/u/7']),
  usePath: vi.fn(() => '/u/7'),
}))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useLocalSessionOnboardingContext: mockUseLocalSessionOnboardingContext,
}))

describe('SetupBanner', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockClear()
    mockDismiss.mockClear()
    mockUseLocalSessionOnboardingContext.mockReturnValue({
      onboarding: {
        dismissed: false,
        dismissedAt: null,
        providerChoiceComplete: false,
        backupComplete: false,
        lockComplete: false,
      },
      loading: false,
      metadataLoaded: true,
      providerChoiceComplete: false,
      isComplete: false,
      dismiss: mockDismiss,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders for incomplete onboarding', () => {
    render(<SetupBanner />)

    expect(screen.getByText('Finish setting up your account')).toBeDefined()
  })

  it('hides after dismissal', () => {
    mockUseLocalSessionOnboardingContext.mockReturnValue({
      onboarding: {
        dismissed: true,
        dismissedAt: 1,
        providerChoiceComplete: true,
        backupComplete: false,
        lockComplete: false,
      },
      loading: false,
      metadataLoaded: true,
      providerChoiceComplete: true,
      isComplete: false,
      dismiss: mockDismiss,
    })

    render(<SetupBanner />)

    expect(screen.queryByText('Finish setting up your account')).toBeNull()
  })

  it('navigates to plan before provider choice is complete', () => {
    render(<SetupBanner />)

    fireEvent.click(screen.getByText('Finish setting up your account'))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7/plan' })
  })

  it('navigates to setup after provider choice when backup is incomplete', () => {
    mockUseLocalSessionOnboardingContext.mockReturnValue({
      onboarding: {
        dismissed: false,
        dismissedAt: null,
        providerChoiceComplete: true,
        backupComplete: false,
        lockComplete: false,
      },
      loading: false,
      metadataLoaded: true,
      providerChoiceComplete: true,
      isComplete: false,
      dismiss: mockDismiss,
    })

    render(<SetupBanner />)

    fireEvent.click(screen.getByText('Finish setting up your account'))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7/setup' })
  })

  it('uses derived provider choice state for linked cloud local sessions', () => {
    mockUseLocalSessionOnboardingContext.mockReturnValue({
      onboarding: {
        dismissed: false,
        dismissedAt: null,
        providerChoiceComplete: false,
        backupComplete: false,
        lockComplete: false,
      },
      loading: false,
      metadataLoaded: true,
      providerChoiceComplete: true,
      isComplete: false,
      dismiss: mockDismiss,
    })

    render(<SetupBanner />)

    fireEvent.click(screen.getByText('Finish setting up your account'))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7/setup' })
  })

  it('stays hidden until session metadata resolves', () => {
    mockUseLocalSessionOnboardingContext.mockReturnValue({
      onboarding: {
        dismissed: false,
        dismissedAt: null,
        providerChoiceComplete: false,
        backupComplete: false,
        lockComplete: false,
      },
      loading: false,
      metadataLoaded: false,
      providerChoiceComplete: false,
      isComplete: false,
      dismiss: mockDismiss,
    })

    render(<SetupBanner />)

    expect(screen.queryByText('Finish setting up your account')).toBeNull()
  })

  it('stores dismissal through the onboarding action', () => {
    render(<SetupBanner />)

    fireEvent.click(screen.getByLabelText('Dismiss setup banner'))

    expect(mockDismiss).toHaveBeenCalledTimes(1)
  })
})
