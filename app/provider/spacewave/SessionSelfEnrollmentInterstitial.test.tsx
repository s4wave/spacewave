import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { SharedObjectSelfEnrollmentErrorCategory } from '@s4wave/sdk/session/shared-object-self-enrollment.pb.js'
import type { WatchSharedObjectSelfEnrollmentStateResponse } from '@s4wave/sdk/session/shared-object-self-enrollment.pb.js'

import { SessionSelfEnrollmentInterstitial } from './SessionSelfEnrollmentInterstitial.js'

interface MockOnboarding {
  sessionSelfEnrollmentCount: number
  sessionSelfEnrollmentGenerationKey: string
}

const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockStart = vi.hoisted(() => vi.fn())
const mockSkip = vi.hoisted(() => vi.fn())
const mockSetSkip = vi.hoisted(() => vi.fn())
const mockState = vi.hoisted<{
  value: WatchSharedObjectSelfEnrollmentStateResponse | undefined
}>(() => ({ value: undefined }))
const mockOnboarding = vi.hoisted<{
  value: MockOnboarding
}>(() => ({
  value: {
    sessionSelfEnrollmentCount: 2,
    sessionSelfEnrollmentGenerationKey: 'gen-1',
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (_resource: unknown, factory: unknown) => {
    const src = String(factory)
    if (src.includes('getSessionInfo')) {
      return {
        value: {
          sessionRef: {
            providerResourceRef: {
              providerId: 'spacewave',
              providerAccountId: 'acct-1',
            },
          },
        },
        loading: false,
        error: null,
        retry: vi.fn(),
      }
    }
    return {
      value: {
        start: mockStart,
        skip: mockSkip,
        watchState: vi.fn(),
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    }
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: mockState.value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({ value: {} }),
  },
  useSessionNavigate: () => mockNavigateSession,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: () => ({ onboarding: mockOnboarding.value }),
  },
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: () => ({
    value: {},
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateAtom: () => [null, mockSetSkip],
}))

vi.mock(
  '@s4wave/app/session/dashboard/AccountDashboardStateContext.js',
  () => ({
    AccountDashboardStateProvider: ({ children }: { children?: ReactNode }) => (
      <>{children}</>
    ),
  }),
)

vi.mock('@s4wave/app/session/dashboard/AuthConfirmDialog.js', () => ({
  AuthConfirmDialog: (props: {
    open: boolean
    onConfirm: (credential: unknown) => Promise<void>
    retainAfterClose?: boolean
  }) =>
    props.open ?
      <button
        data-testid="auth-confirm"
        data-retain={String(!!props.retainAfterClose)}
        onClick={() => void props.onConfirm({ type: 'tracker' })}
      >
        confirm
      </button>
    : null,
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockState.value = undefined
  mockOnboarding.value = {
    sessionSelfEnrollmentCount: 2,
    sessionSelfEnrollmentGenerationKey: 'gen-1',
  }
})

describe('SessionSelfEnrollmentInterstitial', () => {
  it('starts self-enrollment after retained unlock confirmation', async () => {
    render(<SessionSelfEnrollmentInterstitial />)

    fireEvent.click(screen.getByText('Unlock and continue'))
    const confirm = screen.getByTestId('auth-confirm')
    expect(confirm.getAttribute('data-retain')).toBe('true')

    fireEvent.click(confirm)

    await waitFor(() => expect(mockStart).toHaveBeenCalledTimes(1))
  })

  it('calls skip with the current generation and routes to the dashboard', async () => {
    render(<SessionSelfEnrollmentInterstitial />)

    fireEvent.click(screen.getByText('Skip for now'))

    await waitFor(() => expect(mockSkip).toHaveBeenCalledWith('gen-1'))
    expect(mockSetSkip).toHaveBeenCalledWith({
      skippedKey: 'gen-1',
      skippedAt: expect.any(Number),
    })
    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: '/',
      replace: true,
    })
  })

  it('renders running progress and failures from the resource stream', () => {
    mockState.value = {
      count: 2,
      running: true,
      currentSharedObjectId: 'space-2',
      completedSharedObjectIds: ['space-1'],
      failures: [
        {
          sharedObjectId: 'space-3',
          category: SharedObjectSelfEnrollmentErrorCategory.RETRY,
          message: 'temporary failure',
        },
      ],
    }

    render(<SessionSelfEnrollmentInterstitial />)

    expect(screen.getByText('Connecting to 2 spaces')).toBeTruthy()
    expect(screen.getByText('1/2')).toBeTruthy()
    expect(screen.getByText('space-2')).toBeTruthy()
    expect(screen.getByText('temporary failure')).toBeTruthy()
    expect(screen.getByText('Retry now')).toBeTruthy()
  })
})
