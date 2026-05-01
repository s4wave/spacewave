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
import type {
  WatchAccountInfoResponse,
  WatchAuthMethodsResponse,
  WatchEntityKeypairsResponse,
} from '@s4wave/sdk/account/account.pb.js'

import { SessionSelfEnrollmentInterstitial } from './SessionSelfEnrollmentInterstitial.js'

interface MockOnboarding {
  sessionSelfEnrollmentCount: number
  sessionSelfEnrollmentGenerationKey: string
}

const mockNavigateSession = vi.hoisted(() => vi.fn())
const mockEnrollment = vi.hoisted(() => ({
  start: vi.fn((_signal?: AbortSignal) => Promise.resolve({})),
  skip: vi.fn((_generationKey: string) => Promise.resolve({})),
  watchState: vi.fn(),
}))
const mockStart = mockEnrollment.start
const mockSkip = mockEnrollment.skip
const mockSetSkip = vi.hoisted(() => vi.fn())
const mockState = vi.hoisted<{
  value: WatchSharedObjectSelfEnrollmentStateResponse | undefined
}>(() => ({ value: undefined }))
const mockAccountState = vi.hoisted<{
  accountInfo: {
    value: WatchAccountInfoResponse
    loading: boolean
    error: null
    retry: ReturnType<typeof vi.fn>
  }
  authMethods: {
    value: WatchAuthMethodsResponse
    loading: boolean
    error: null
    retry: ReturnType<typeof vi.fn>
  }
  entityKeypairs: {
    value: WatchEntityKeypairsResponse
    loading: boolean
    error: null
    retry: ReturnType<typeof vi.fn>
  }
}>(() => ({
  accountInfo: {
    value: { authThreshold: 0 },
    loading: false,
    error: null,
    retry: vi.fn(),
  },
  authMethods: {
    value: { authMethods: [] },
    loading: false,
    error: null,
    retry: vi.fn(),
  },
  entityKeypairs: {
    value: { keypairs: [], unlockedCount: 0 },
    loading: false,
    error: null,
    retry: vi.fn(),
  },
}))
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
      value: mockEnrollment,
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
    useAccountDashboardState: () => mockAccountState,
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
  mockAccountState.accountInfo.value = { authThreshold: 0 }
  mockAccountState.authMethods.value = { authMethods: [] }
  mockAccountState.entityKeypairs.value = { keypairs: [], unlockedCount: 0 }
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
      skippedAt: expect.any(Number) as number,
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

  it('shows retry controls when enrollment stops with failures', () => {
    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: false,
      currentSharedObjectId: '',
      completedSharedObjectIds: ['space-1'],
      failures: [
        {
          sharedObjectId: 'space-2',
          category: SharedObjectSelfEnrollmentErrorCategory.REPORT,
          message: 'recovery envelope not found',
        },
      ],
    }
    mockAccountState.entityKeypairs.value = {
      keypairs: [{ keypair: { peerId: 'peer-1' }, unlocked: true }],
      unlockedCount: 1,
    }

    render(<SessionSelfEnrollmentInterstitial />)

    expect(screen.queryByText('Connecting to 3 spaces')).toBeNull()
    expect(screen.getByText('recovery envelope not found')).toBeTruthy()
    expect(screen.getByText('Unlock and continue')).toBeTruthy()
    expect(screen.getByText('Skip for now')).toBeTruthy()
  })

  it('keeps the initial total when pending count shrinks during enrollment', () => {
    mockOnboarding.value = {
      sessionSelfEnrollmentCount: 6,
      sessionSelfEnrollmentGenerationKey: 'gen-1',
    }
    mockState.value = {
      count: 4,
      running: true,
      currentSharedObjectId: 'space-5',
      completedSharedObjectIds: [],
      failures: [],
    }

    render(<SessionSelfEnrollmentInterstitial />)

    expect(screen.getByText('Connecting to 6 spaces')).toBeTruthy()
    expect(screen.getByText('2/6')).toBeTruthy()
  })

  it('starts enrollment when enough signers are already unlocked', async () => {
    mockOnboarding.value = {
      sessionSelfEnrollmentCount: 3,
      sessionSelfEnrollmentGenerationKey: 'gen-1',
    }
    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: false,
      completedSharedObjectIds: [],
      failures: [],
    }
    mockAccountState.entityKeypairs.value = {
      keypairs: [{ keypair: { peerId: 'peer-1' }, unlocked: true }],
      unlockedCount: 1,
    }

    render(<SessionSelfEnrollmentInterstitial />)

    expect(screen.getByText('Connecting to 3 spaces')).toBeTruthy()
    expect(screen.getByText('0/3')).toBeTruthy()
    expect(screen.queryByText('Unlock and continue')).toBeNull()
    expect(screen.queryByText('Skip for now')).toBeNull()
    await waitFor(() => expect(mockStart).toHaveBeenCalledTimes(1))
  })

  it('does not abort the start signal on intermediate stream updates', async () => {
    mockOnboarding.value = {
      sessionSelfEnrollmentCount: 3,
      sessionSelfEnrollmentGenerationKey: 'gen-1',
    }
    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: false,
      completedSharedObjectIds: [],
      failures: [],
    }
    mockAccountState.entityKeypairs.value = {
      keypairs: [{ keypair: { peerId: 'peer-1' }, unlocked: true }],
      unlockedCount: 1,
    }

    const { rerender } = render(<SessionSelfEnrollmentInterstitial />)
    await waitFor(() => expect(mockStart).toHaveBeenCalledTimes(1))
    const signal = mockStart.mock.calls[0]?.[0]
    expect(signal?.aborted).toBe(false)

    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: true,
      currentSharedObjectId: 'space-1',
      completedSharedObjectIds: [],
      failures: [],
    }
    rerender(<SessionSelfEnrollmentInterstitial />)
    expect(mockStart).toHaveBeenCalledTimes(1)
    expect(signal?.aborted).toBe(false)

    mockState.value = {
      count: 2,
      generationKey: 'gen-1',
      running: true,
      currentSharedObjectId: 'space-2',
      completedSharedObjectIds: ['space-1'],
      failures: [],
    }
    rerender(<SessionSelfEnrollmentInterstitial />)
    expect(mockStart).toHaveBeenCalledTimes(1)
    expect(signal?.aborted).toBe(false)
  })

  it('shows retry controls when an auto-started run stops before all spaces connect', async () => {
    mockOnboarding.value = {
      sessionSelfEnrollmentCount: 3,
      sessionSelfEnrollmentGenerationKey: 'gen-1',
    }
    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: false,
      completedSharedObjectIds: [],
      failures: [],
    }
    mockAccountState.entityKeypairs.value = {
      keypairs: [{ keypair: { peerId: 'peer-1' }, unlocked: true }],
      unlockedCount: 1,
    }

    const { rerender } = render(<SessionSelfEnrollmentInterstitial />)
    await waitFor(() => expect(mockStart).toHaveBeenCalledTimes(1))

    mockState.value = {
      count: 3,
      generationKey: 'gen-1',
      running: false,
      completedSharedObjectIds: ['space-1', 'space-2'],
      failures: [],
    }
    rerender(<SessionSelfEnrollmentInterstitial />)

    expect(screen.queryByText('Connecting to 3 spaces')).toBeNull()
    expect(screen.queryByText('2/3')).toBeNull()
    expect(screen.getByText('Unlock and continue')).toBeTruthy()
    expect(screen.getByText('Skip for now')).toBeTruthy()
  })
})
