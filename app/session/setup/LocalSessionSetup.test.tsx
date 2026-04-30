import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import superjson from 'superjson'
import { LocalSessionSetup, WarningCard } from './LocalSessionSetup.js'
import { localSessionOnboardingStoreId } from './local-session-onboarding-state.js'

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: vi.fn(() => vi.fn()),
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: vi.fn(() => ({ value: null })) },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: vi.fn(() => null),
}))

const mockUseRootResource = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

const mockSetOnboarding = vi.hoisted(() => vi.fn())
const mockOnboardingLoading = vi.hoisted(() => vi.fn(() => false))

vi.mock('@s4wave/app/session/setup/LocalSessionOnboardingContext.js', () => ({
  useLocalSessionOnboardingContext: vi.fn(() => ({
    loading: mockOnboardingLoading(),
    setOnboarding: mockSetOnboarding,
  })),
}))

vi.mock('@s4wave/web/state/index.js', () => ({
  useStateNamespace: vi.fn(() => ({ stateAtomAccessor: { loading: false } })),
}))

const mockUsePromise = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: mockUsePromise,
}))

import { useNavigate } from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

const mockUseNavigate = useNavigate as ReturnType<typeof vi.fn>
const mockUseResourceValue = useResourceValue as ReturnType<typeof vi.fn>

describe('LocalSessionSetup', () => {
  const mockNavigate = vi.fn()
  const mockGetLinkedLocalSession = vi.fn()
  const mockCreateLinkedLocalSession = vi.fn()
  const mockGetState = vi.fn()
  const mockSetState = vi.fn()
  const mockAccessStateAtom = vi.fn()
  const mockMountSessionByIdx = vi.fn()

  beforeEach(() => {
    cleanup()
    mockNavigate.mockClear()
    mockSetOnboarding.mockClear()
    mockOnboardingLoading.mockReset()
    mockOnboardingLoading.mockReturnValue(false)
    mockUsePromise.mockReset()
    mockUsePromise.mockImplementation(() => undefined)
    mockGetLinkedLocalSession.mockReset()
    mockCreateLinkedLocalSession.mockReset()
    mockGetState.mockReset()
    mockSetState.mockReset()
    mockAccessStateAtom.mockReset()
    mockMountSessionByIdx.mockReset()
    mockUseResourceValue.mockReturnValue(null)
    mockUseRootResource.mockReturnValue({ value: null })
    mockUseNavigate.mockReturnValue(mockNavigate)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('local mode rendering', () => {
    it('renders the header text', () => {
      render(<LocalSessionSetup mode="local" />)
      expect(screen.getByText('Preparing local storage')).toBeDefined()
    })

    it('renders the subtitle', () => {
      render(<LocalSessionSetup mode="local" />)
      expect(
        screen.getByText('Opening your full local setup flow.'),
      ).toBeDefined()
    })

    it('renders the animated logo', () => {
      render(<LocalSessionSetup mode="local" />)
      expect(screen.getByTestId('animated-logo')).toBeDefined()
    })

    it('does not render the duplicate continue button', () => {
      render(<LocalSessionSetup mode="local" />)
      expect(screen.queryByText('Continue to Spacewave')).toBeNull()
    })

    it('dismisses the banner before redirecting back into setup', () => {
      mockUsePromise.mockImplementation((cb: () => void) => cb())

      render(<LocalSessionSetup mode="local" />)

      expect(mockSetOnboarding).toHaveBeenCalledTimes(1)
      const update = mockSetOnboarding.mock.calls[0][0] as (prev: {
        dismissed: boolean
        dismissedAt: number | null
        providerChoiceComplete: boolean
        backupComplete: boolean
        lockComplete: boolean
      }) => {
        dismissed: boolean
        dismissedAt: number | null
        providerChoiceComplete: boolean
        backupComplete: boolean
        lockComplete: boolean
      }
      const next = update({
        dismissed: false,
        dismissedAt: null,
        providerChoiceComplete: false,
        backupComplete: false,
        lockComplete: false,
      })

      expect(next.providerChoiceComplete).toBe(true)
      expect(next.dismissed).toBe(true)
      expect(typeof next.dismissedAt).toBe('number')
      expect(mockNavigate).toHaveBeenCalledWith({ path: '../' })
    })

    it('waits for the first onboarding snapshot before redirecting', () => {
      mockOnboardingLoading.mockReturnValue(true)
      mockUsePromise.mockImplementation((cb: () => void) => cb())

      render(<LocalSessionSetup mode="local" />)

      expect(mockSetOnboarding).not.toHaveBeenCalled()
      expect(mockNavigate).not.toHaveBeenCalled()
    })
  })

  describe('cloud mode rendering', () => {
    it('renders the header text', () => {
      render(
        <LocalSessionSetup
          mode="cloud"
          metadata={{ providerId: 'spacewave' }}
        />,
      )
      expect(screen.getByText('Preparing local storage')).toBeDefined()
    })

    it('waits for session metadata before taking the local redirect fallback', () => {
      render(<LocalSessionSetup mode="cloud" />)

      expect(mockNavigate).not.toHaveBeenCalled()
      expect(screen.getByText('Preparing local storage')).toBeDefined()
    })

    it('does not render the duplicate continue button', () => {
      render(
        <LocalSessionSetup
          mode="cloud"
          metadata={{ providerId: 'spacewave' }}
        />,
      )
      expect(screen.queryByText('Continue to Spacewave')).toBeNull()
    })

    it('dismisses the linked local banner before navigating back to it', async () => {
      mockUsePromise.mockImplementation(
        (cb: (signal: AbortSignal) => Promise<void> | undefined) => {
          void cb(new AbortController().signal)
          return undefined
        },
      )
      mockUseResourceValue.mockReturnValue({
        spacewave: {
          getLinkedLocalSession: mockGetLinkedLocalSession,
          createLinkedLocalSession: mockCreateLinkedLocalSession,
        },
      })
      mockUseRootResource.mockReturnValue({
        value: {
          mountSessionByIdx: mockMountSessionByIdx,
        },
      })
      mockGetLinkedLocalSession.mockResolvedValue({
        found: true,
        sessionIndex: 7,
      })
      mockMountSessionByIdx.mockResolvedValue({
        session: {
          accessStateAtom: mockAccessStateAtom,
          [Symbol.dispose]: vi.fn(),
        },
      })
      mockAccessStateAtom.mockResolvedValue({
        getState: mockGetState,
        setState: mockSetState,
        [Symbol.dispose]: vi.fn(),
      })
      mockGetState.mockResolvedValue({
        stateJson: superjson.stringify({
          dismissed: false,
          dismissedAt: null,
          providerChoiceComplete: false,
          backupComplete: true,
          lockComplete: false,
        }),
      })

      render(
        <LocalSessionSetup
          mode="cloud"
          metadata={{ providerId: 'spacewave' }}
        />,
      )

      await waitFor(() => {
        expect(mockMountSessionByIdx).toHaveBeenCalledWith(
          { sessionIdx: 7 },
          expect.any(AbortSignal),
        )
      })
      expect(mockAccessStateAtom).toHaveBeenCalledWith(
        { storeId: localSessionOnboardingStoreId },
        expect.any(AbortSignal),
      )
      expect(mockSetState).toHaveBeenCalledTimes(1)
      const [stateJson] = mockSetState.mock.calls[0] as [string]
      const next = superjson.parse<{
        dismissed: boolean
        dismissedAt: number | null
        providerChoiceComplete: boolean
        backupComplete: boolean
        lockComplete: boolean
      }>(stateJson)

      expect(next.providerChoiceComplete).toBe(true)
      expect(next.dismissed).toBe(true)
      expect(typeof next.dismissedAt).toBe('number')
      expect(next.backupComplete).toBe(true)
      expect(next.lockComplete).toBe(false)
      expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7/setup' })
    })
  })
})

describe('WarningCard', () => {
  beforeEach(() => {
    cleanup()
  })

  afterEach(() => {
    cleanup()
  })

  it('marks the desktop app download as complete in desktop builds', () => {
    const onDownload = vi.fn()

    render(<WarningCard onDownload={onDownload} onUpgrade={vi.fn()} />)

    const download = screen.getByRole('button', {
      name: /Download the desktop app/i,
    })
    expect(download).toHaveProperty('disabled', true)

    fireEvent.click(download)

    expect(onDownload).not.toHaveBeenCalled()
  })
})
