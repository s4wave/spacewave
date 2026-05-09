import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import { TargetedInvitePurpose } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { SessionContainer } from './SessionContainer.js'

const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUseBottomBarSetOpenMenu = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseStreamingResource = vi.hoisted(() => vi.fn())
const mockConsumePendingJoin = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseSessionIndex = vi.hoisted(() => vi.fn())
const mockUsePath = vi.hoisted(() => vi.fn())
const mockSessionSelfEnrollmentProvider = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: vi.fn(),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: mockUseStreamingResource,
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/app/routes/pendingJoin.js', () => ({
  consumePendingJoin: mockConsumePendingJoin,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  Route: () => null,
  Routes: ({ children }: { children?: ReactNode }) => <>{children}</>,
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
  useParentPaths: () => [],
  usePath: mockUsePath,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    Provider: ({ children }: { children?: ReactNode }) => <>{children}</>,
    useContext: () => ({ value: null }),
  },
  useSessionIndex: mockUseSessionIndex,
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: () => null,
}))

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: (props: {
    id: string
    position?: 'left' | 'right'
    button?: (
      selected: boolean,
      onClick: () => void,
      className?: string,
    ) => ReactNode
    overlay?: ReactNode
    children?: ReactNode
  }) => {
    const isAccount = props.id === 'account'
    return (
      <div
        data-testid={`bottom-bar-level-${props.id}`}
        data-position={props.position ?? 'left'}
      >
        <div data-testid={`bottom-bar-button-${props.id}`}>
          {props.button?.(false, () => {}, '')}
        </div>
        <div data-testid={isAccount ? 'overlay' : `overlay-${props.id}`}>
          {isAccount ? props.overlay : null}
        </div>
        <div data-testid={isAccount ? 'content' : `content-${props.id}`}>
          {props.children}
        </div>
      </div>
    )
  },
}))

vi.mock('@s4wave/web/frame/bottom-bar-item.js', () => ({
  BottomBarItem: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: mockUseBottomBarSetOpenMenu,
}))

vi.mock('@s4wave/web/frame/bottom-icon-props.js', () => ({
  bottomBarIconProps: {},
}))

vi.mock('@s4wave/web/state/index.js', () => ({
  StateNamespaceProvider: ({ children }: { children?: ReactNode }) => children,
  useStateNamespace: () => ['session'],
}))

vi.mock('@aptre/bldr-react', () => ({
  DebugInfo: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('./SessionCommands.js', () => ({
  SessionCommands: () => null,
}))

vi.mock('./SessionSelfEnrollmentStatusContext.js', () => ({
  SessionSelfEnrollmentStatusProvider: ({
    children,
  }: {
    children?: ReactNode
  }) => {
    mockSessionSelfEnrollmentProvider()
    return <div data-testid="session-self-enrollment-provider">{children}</div>
  },
  useSessionSelfEnrollmentStatus: () => ({
    resource: null,
    snapshot: null,
    visualState: 'ready',
    loading: false,
    pending: false,
    running: false,
    credentialRequired: false,
    skipped: false,
    failed: false,
    count: 0,
    completedCount: 0,
    totalCount: 0,
    progress: 0,
    generationKey: '',
    failures: [],
    summaryLabel: '',
    detailLabel: '',
  }),
}))

vi.mock('./dashboard/SessionDetails.js', () => ({
  SessionDetails: () => <div data-testid="session-details" />,
}))

vi.mock('./PinUnlockOverlay.js', () => ({
  PinUnlockOverlay: () => <div data-testid="pin-unlock-overlay" />,
}))

vi.mock('@s4wave/app/provider/spacewave/SpacewaveSessionContent.js', () => ({
  SpacewaveSessionContent: ({
    children,
    onboarding,
  }: {
    children?: ReactNode
    onboarding?: { accountStatus?: ProviderAccountStatus } | null
  }) => (
    <div
      data-testid="spacewave-content"
      data-account-status={onboarding?.accountStatus ?? ''}
    >
      {children}
    </div>
  ),
}))

vi.mock('@s4wave/app/provider/local/LocalSessionContent.js', () => ({
  LocalSessionContent: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/app/provider/spacewave/SpacewaveRootRouter.js', () => ({
  SpacewaveRootRouter: () => null,
}))

vi.mock('./SessionDashboardContainer.js', () => ({
  SessionDashboardContainer: () => null,
}))

vi.mock('./SessionSharedObjectContainer.js', () => ({
  SessionSharedObjectContainer: () => null,
}))

vi.mock('./SetupWizard.js', () => ({
  SetupWizard: () => null,
}))

vi.mock('./setup/ProviderSetup.js', () => ({
  ProviderSetup: () => null,
}))

vi.mock('./setup/LocalSessionSetup.js', () => ({
  LocalSessionSetup: () => null,
}))

vi.mock('./setup/LinkDeviceWizard.js', () => ({
  LinkDeviceWizard: () => null,
}))

vi.mock('./settings/TransferWizard.js', () => ({
  TransferWizard: () => null,
}))

vi.mock('@s4wave/app/billing/BillingCancelRoute.js', () => ({
  BillingCancelRoute: () => null,
}))

vi.mock('@s4wave/app/billing/BillingAccountsRoute.js', () => ({
  BillingAccountsRoute: () => null,
}))

vi.mock('@s4wave/app/billing/BillingAccountDetailRoute.js', () => ({
  BillingAccountDetailRoute: () => null,
}))

vi.mock('@s4wave/app/org/OrgContainer.js', () => ({
  OrgContainer: () => null,
}))

vi.mock('@s4wave/app/sobject/JoinSpacePage.js', () => ({
  JoinSpacePage: () => null,
}))

vi.mock('@s4wave/app/pair/PairCodePage.js', () => ({
  PairCodePage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/SpacewaveSessionRoutes.js', () => ({
  spacewaveSessionRoutes: () => null,
}))

vi.mock('./DeletedAccountOverlay.js', () => ({
  DeletedAccountOverlay: (props: { metadata: { displayName?: string } }) => (
    <div data-testid="deleted-account-overlay">
      {props.metadata.displayName ?? ''}
    </div>
  ),
}))

vi.mock('./DormantOverlay.js', () => ({
  DormantOverlay: () => <div data-testid="dormant-overlay" />,
}))

vi.mock('./ReAuthOverlay.js', () => ({
  ReAuthOverlay: (props: { metadata: { displayName?: string } }) => (
    <div data-testid="reauth-overlay">{props.metadata.displayName ?? ''}</div>
  ),
}))

describe('SessionContainer', () => {
  type StreamingResourceCall = [
    { value: unknown },
    (value: unknown, signal: AbortSignal) => unknown,
    unknown?,
  ]

  function mockStreams(
    lockState: unknown,
    onboardingState: unknown,
    inboxState: unknown = { invitations: [] },
  ) {
    mockUseStreamingResource
      .mockReturnValueOnce({ value: onboardingState })
      .mockReturnValueOnce({ value: lockState })
      .mockReturnValueOnce({ value: inboxState })
      .mockReturnValue({ value: inboxState })
  }

  it('wraps the routes tree with spacewave session providers and renders session details in the overlay', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockSessionSelfEnrollmentProvider.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {})
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    const overlay = screen.getByTestId('overlay')
    const content = screen.getByTestId('content')
    expect(within(content).getByTestId('spacewave-content')).toBeTruthy()
    expect(
      within(content).getByTestId('session-self-enrollment-provider'),
    ).toBeTruthy()
    expect(mockSessionSelfEnrollmentProvider).toHaveBeenCalledTimes(1)
    expect(within(overlay).queryByTestId('spacewave-content')).toBeNull()
    expect(within(overlay).getByTestId('session-details')).toBeTruthy()
  })

  it('opens the Onboarding Status route-status stream at the session container boundary', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockSessionSelfEnrollmentProvider.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
    })
    mockConsumePendingJoin.mockReturnValue(null)
    const watchOnboardingStatus = vi.fn(() => 'onboarding-stream')
    const watchLockState = vi.fn(() => 'lock-stream')
    const session = {
      spacewave: { watchOnboardingStatus },
      watchLockState,
    }
    const signal = new AbortController().signal

    render(
      <SessionContainer
        sessionResource={{
          value: session as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(mockUseStreamingResource.mock.calls.length).toBeGreaterThanOrEqual(2)
    const streamingCalls = mockUseStreamingResource.mock
      .calls as StreamingResourceCall[]
    const [[onboardingResource, onboardingReader], [lockResource, lockReader]] =
      streamingCalls
    expect(onboardingResource.value).toBe(session)
    expect(onboardingReader(session, signal)).toBe('onboarding-stream')
    expect(watchOnboardingStatus).toHaveBeenCalledWith(signal)
    expect(lockResource.value).toBe(session)
    expect(lockReader(session, signal)).toBe('lock-stream')
    expect(watchLockState).toHaveBeenCalledWith({}, signal)
    expect(
      screen
        .getByTestId('spacewave-content')
        .getAttribute('data-account-status'),
    ).toBe(String(ProviderAccountStatus.ProviderAccountStatus_READY))
  })

  it('keeps local sessions off the Onboarding Status route-status stream', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockSessionSelfEnrollmentProvider.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, null)
    mockConsumePendingJoin.mockReturnValue(null)
    const session = {
      spacewave: { watchOnboardingStatus: vi.fn() },
      watchLockState: vi.fn(),
    }

    render(
      <SessionContainer
        sessionResource={{
          value: session as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'local',
          displayName: 'Local Session',
        }}
      />,
    )

    const streamingCalls = mockUseStreamingResource.mock
      .calls as StreamingResourceCall[]
    const [[onboardingResource]] = streamingCalls
    expect(onboardingResource.value).toBeNull()
    expect(session.spacewave.watchOnboardingStatus).not.toHaveBeenCalled()
    expect(screen.queryByTestId('spacewave-content')).toBeNull()
    expect(screen.queryByTestId('session-self-enrollment-provider')).toBeNull()
    expect(mockSessionSelfEnrollmentProvider).not.toHaveBeenCalled()
  })

  it('renders the local account overlay without any session provider chrome inside it', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, null)
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'local',
          displayName: 'Local Session',
        }}
      />,
    )

    const overlay = screen.getByTestId('overlay')
    expect(within(overlay).queryByTestId('spacewave-content')).toBeNull()
    expect(within(overlay).getByTestId('session-details')).toBeTruthy()
  })

  it('waits for the root resource before removing a deleted session', async () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    const deleteSession = vi.fn(async () => {})
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_DELETED,
    })
    mockConsumePendingJoin.mockReturnValue(null)

    const { rerender } = render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    await waitFor(() => {
      expect(deleteSession).not.toHaveBeenCalled()
      expect(mockNavigate).not.toHaveBeenCalled()
    })

    mockUseRootResource.mockReturnValue({
      value: { deleteSession },
    })
    mockUseStreamingResource.mockReset()
    mockStreams(null, {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_DELETED,
    })
    rerender(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    await waitFor(() => {
      expect(deleteSession).toHaveBeenCalledWith(1)
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/sessions',
        replace: true,
      })
    })
  })

  it('renders the deleted account overlay instead of the routed session chrome', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1/so/space-id')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_DELETED,
    })
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Deleted Cloud Session',
        }}
      />,
    )

    expect(screen.getByTestId('deleted-account-overlay').textContent).toBe(
      'Deleted Cloud Session',
    )
    expect(screen.queryByTestId('content')).toBeNull()
    expect(screen.queryByTestId('pin-unlock-overlay')).toBeNull()
  })

  it('renders the reauthentication overlay for unauthenticated cloud sessions', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      accountStatus:
        ProviderAccountStatus.ProviderAccountStatus_UNAUTHENTICATED,
    })
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Signed-out Cloud Session',
        }}
      />,
    )

    expect(screen.getByTestId('reauth-overlay').textContent).toBe(
      'Signed-out Cloud Session',
    )
    expect(screen.queryByTestId('content')).toBeNull()
    expect(screen.queryByTestId('pin-unlock-overlay')).toBeNull()
  })

  it('renders the dormant overlay for dormant cloud sessions outside /plan routes', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      setupRequired: true,
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_DORMANT,
    })
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(screen.getByTestId('dormant-overlay')).toBeTruthy()
  })

  it('allows dormant /plan routes through so upgrade routing can render', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/plan/upgrade')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {
      setupRequired: true,
      accountStatus: ProviderAccountStatus.ProviderAccountStatus_DORMANT,
    })
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(screen.queryByTestId('dormant-overlay')).toBeNull()
    expect(screen.getByTestId('content')).toBeTruthy()
  })

  it('renders the mounted unlock overlay when the session locks in place', () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(
      { mode: SessionLockMode.PIN_ENCRYPTED, locked: true },
      {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      },
    )
    mockConsumePendingJoin.mockReturnValue(null)

    render(
      <SessionContainer
        sessionResource={{
          value: {
            unlockSession: vi.fn(),
          } as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(screen.getByTestId('pin-unlock-overlay')).toBeTruthy()
    expect(screen.queryByTestId('content')).toBeNull()
  })

  it('navigates to a stashed join code once the session is mounted', async () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(null, {})
    mockConsumePendingJoin.mockReturnValue('abc123')

    render(
      <SessionContainer
        sessionResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith({
        path: './join/abc123',
        replace: true,
      })
    })
  })

  it('renders targeted space invites from the watched inbox and accepts them', async () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(
      null,
      {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      },
      {
        invitations: [
          {
            id: 'invite-1',
            status: 'pending',
            purpose: TargetedInvitePurpose.SPACE,
            contextId: 'space-1',
            role: 'writer',
            actorAccountId: 'acct-owner',
          },
        ],
      },
    )
    mockConsumePendingJoin.mockReturnValue(null)
    const acceptSpaceTargetedInvitation = vi.fn(async () => {})
    const session = {
      spacewave: {
        watchOnboardingStatus: vi.fn(),
        watchTargetedInvitations: vi.fn(),
        acceptSpaceTargetedInvitation,
      },
      watchLockState: vi.fn(),
    }

    render(
      <SessionContainer
        sessionResource={{
          value: session as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(screen.getByText('Pending Invites')).toBeTruthy()
    expect(screen.getByText('Space invite: space-1')).toBeTruthy()
    fireEvent.click(screen.getByText('Accept'))

    await waitFor(() => {
      expect(acceptSpaceTargetedInvitation).toHaveBeenCalledWith('invite-1')
    })
  })

  it('declines targeted invites through the watched inbox', async () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(
      null,
      {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      },
      {
        invitations: [
          {
            id: 'invite-1',
            status: 'pending',
            purpose: TargetedInvitePurpose.SPACE,
            contextId: 'space-1',
            role: 'writer',
            actorAccountId: 'acct-owner',
          },
        ],
      },
    )
    mockConsumePendingJoin.mockReturnValue(null)
    const processTargetedInvitation = vi.fn(async () => {})
    const session = {
      spacewave: {
        watchOnboardingStatus: vi.fn(),
        watchTargetedInvitations: vi.fn(),
        processTargetedInvitation,
      },
      watchLockState: vi.fn(),
    }

    render(
      <SessionContainer
        sessionResource={{
          value: session as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    fireEvent.click(screen.getByText('Decline'))

    await waitFor(() => {
      expect(processTargetedInvitation).toHaveBeenCalledWith(
        'invite-1',
        'decline',
      )
    })
  })

  it('accepts targeted organization invites from the watched inbox', async () => {
    cleanup()
    mockNavigate.mockReset()
    mockUseStreamingResource.mockReset()
    mockUseSessionIndex.mockReturnValue(1)
    mockUsePath.mockReturnValue('/u/1')
    mockUseSessionInfo.mockReturnValue({
      peerId: 'peer-id',
    })
    mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
    mockUseRootResource.mockReturnValue({ value: null })
    mockStreams(
      null,
      {
        accountStatus: ProviderAccountStatus.ProviderAccountStatus_READY,
      },
      {
        invitations: [
          {
            id: 'invite-1',
            status: 'pending',
            purpose: TargetedInvitePurpose.ORGANIZATION,
            contextId: 'org-1',
            role: 'org:member',
            actorAccountId: 'acct-owner',
          },
        ],
      },
    )
    mockConsumePendingJoin.mockReturnValue(null)
    const acceptOrganizationTargetedInvitation = vi.fn(async () => {})
    const session = {
      spacewave: {
        watchOnboardingStatus: vi.fn(),
        watchTargetedInvitations: vi.fn(),
        acceptOrganizationTargetedInvitation,
      },
      watchLockState: vi.fn(),
    }

    render(
      <SessionContainer
        sessionResource={{
          value: session as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        metadata={{
          providerId: 'spacewave',
          displayName: 'Cloud Session',
        }}
      />,
    )

    expect(screen.getByText('Organization invite: org-1')).toBeTruthy()
    fireEvent.click(screen.getByText('Accept'))

    await waitFor(() => {
      expect(acceptOrganizationTargetedInvitation).toHaveBeenCalledWith(
        'invite-1',
      )
    })
  })

  it.each(['/u/1', '/u/1/so/space-id', '/u/1/org/org-id', '/u/1/billing'])(
    'registers session status buttons once for %s',
    (path) => {
      cleanup()
      mockNavigate.mockReset()
      mockUseStreamingResource.mockReset()
      mockUseSessionIndex.mockReturnValue(1)
      mockUsePath.mockReturnValue(path)
      mockUseSessionInfo.mockReturnValue({
        peerId: 'peer-id',
      })
      mockUseBottomBarSetOpenMenu.mockReturnValue(undefined)
      mockUseRootResource.mockReturnValue({ value: null })
      mockStreams(null, {})
      mockConsumePendingJoin.mockReturnValue(null)

      render(
        <SessionContainer
          sessionResource={{
            value: null,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
          metadata={{
            providerId: 'spacewave',
            displayName: 'Cloud Session',
          }}
        />,
      )

      expect(
        screen.getAllByTestId('bottom-bar-level-system-status'),
      ).toHaveLength(1)
      expect(
        screen.getAllByTestId('bottom-bar-level-session-sync-status'),
      ).toHaveLength(1)
      expect(
        screen
          .getByTestId('bottom-bar-level-system-status')
          .getAttribute('data-position'),
      ).toBe('right')
      expect(
        screen
          .getByTestId('bottom-bar-level-session-sync-status')
          .getAttribute('data-position'),
      ).toBe('right')
      const statusItems = within(screen.getByTestId('content')).getAllByTestId(
        /bottom-bar-level-(session-sync-status|system-status)/,
      )
      expect(
        statusItems.map((item) => item.getAttribute('data-testid')),
      ).toEqual([
        'bottom-bar-level-session-sync-status',
        'bottom-bar-level-system-status',
      ])
    },
  )
})
