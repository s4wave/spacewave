import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'

import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Session } from '@s4wave/sdk/session/session.js'
import {
  Route,
  Routes,
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { SessionDashboardContainer } from './SessionDashboardContainer.js'
import { SessionSharedObjectContainer } from './SessionSharedObjectContainer.js'
import { SetupWizard } from './SetupWizard.js'
import { ProviderSetup } from './setup/ProviderSetup.js'
import { LocalSessionSetup } from './setup/LocalSessionSetup.js'
import { LinkDeviceWizard } from './setup/LinkDeviceWizard.js'
import { CommandLineSetupPage } from './settings/CommandLineSetupPage.js'
import { CliTerminalPage } from './settings/CliTerminalPage.js'
import { TransferWizard } from './settings/TransferWizard.js'
import { BillingAccountDetailRoute } from '@s4wave/app/billing/BillingAccountDetailRoute.js'
import { BillingAccountsRoute } from '@s4wave/app/billing/BillingAccountsRoute.js'
import { BillingCancelRoute } from '@s4wave/app/billing/BillingCancelRoute.js'
import { OrgContainer } from '@s4wave/app/org/OrgContainer.js'
import { JoinSpacePage } from '@s4wave/app/sobject/JoinSpacePage.js'
import { consumePendingJoin } from '@s4wave/app/routes/pendingJoin.js'
import { CreateSpaceRoute } from '@s4wave/app/quickstart/CreateSpaceRoute.js'
import { PairCodePage } from '@s4wave/app/pair/PairCodePage.js'
import { SpacewaveRootRouter } from '@s4wave/app/provider/spacewave/SpacewaveRootRouter.js'
import { spacewaveSessionRoutes } from '@s4wave/app/provider/spacewave/SpacewaveSessionRoutes.js'
import { SessionProviderContainer } from './SessionProviderContainer.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { bottomBarIconProps } from '@s4wave/web/frame/bottom-icon-props.js'
import {
  LuArrowLeft,
  LuArrowUp,
  LuCompass,
  LuCheck,
  LuInbox,
  LuPersonStanding,
  LuX,
} from 'react-icons/lu'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  StateNamespaceProvider,
  useStateNamespace,
  type StateAtomAccessor,
} from '@s4wave/web/state/index.js'
import { type Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import { SessionCommands } from './SessionCommands.js'
import { SessionDetails } from './dashboard/SessionDetails.js'
import { DeletedAccountOverlay } from './DeletedAccountOverlay.js'
import { DormantOverlay } from './DormantOverlay.js'
import { ReAuthOverlay } from './ReAuthOverlay.js'
import { SessionFlowFrame } from './SessionFlowFrame.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import type { ReauthenticateSessionRequest } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { DebugInfo } from '@aptre/bldr-react'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { PinUnlockOverlay } from './PinUnlockOverlay.js'
import {
  SessionLockMode,
  type EntityCredential,
} from '@s4wave/core/session/session.pb.js'
import { SystemStatusButton } from '@s4wave/app/system/SystemStatusButton.js'
import { RecoveryStatusPublisher } from '@s4wave/app/system/RecoveryStatusPublisher.js'
import { SessionSyncStatusButton } from './SessionSyncStatusButton.js'
import { SessionSyncStatusProvider } from './SessionSyncStatusContext.js'
import { SessionSelfEnrollmentStatusButton } from './SessionSelfEnrollmentStatusButton.js'
import { SessionSelfEnrollmentStatusProvider } from './SessionSelfEnrollmentStatusContext.js'
import {
  SessionUploadManagerProvider,
  SessionUploadIndicator,
} from './SessionUploadManagerContext.js'
import {
  TargetedInvitePurpose,
  type TargetedInvitationInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// SessionInfoDebug displays the session info as JSON for debugging.
export function SessionInfoDebug(props: { session: Session }) {
  const {
    data: sessionInfo,
    loading,
    error,
  } = usePromise(
    useCallback(() => props.session.getSessionInfo(), [props.session]),
  )

  if (loading) {
    return (
      <div className="flex items-center p-2">
        <LoadingInline label="Loading session info" tone="muted" size="sm" />
      </div>
    )
  }

  if (error) {
    return <div>Error loading session info: {error.message}</div>
  }

  return <div>Session mounted: {JSON.stringify(sessionInfo)}</div>
}

// SessionRootRouter handles the root route redirect logic for a session.
// Dispatches to SpacewaveRootRouter for cloud sessions.
// Local sessions show the dashboard directly.
function SessionRootRouter(props: { metadata?: SessionMetadata }) {
  if (props.metadata?.providerId === 'spacewave') {
    return <SpacewaveRootRouter />
  }

  return <SessionDashboardContainer />
}

// SessionContainer is the top level entrypoint for URL routing for a Session.
// It wraps routes with BottomBarLevel to register the account menu item.
// Nested routes register their own items, creating a hierarchical bottom bar.
export function SessionContainer(props: {
  sessionResource: Resource<Session>
  metadata?: SessionMetadata
}) {
  const session = props.sessionResource.value

  // Signal to bootstrap.ts that this user has product state to return to.
  // Return visitors with hasSession see the loading screen instead of landing.
  useEffect(() => {
    if (session && !localStorage.getItem('spacewave-has-session')) {
      localStorage.setItem('spacewave-has-session', '1')
    }
  }, [session])

  // Pick up a pending join code stashed by JoinRedirect (no-session path).
  const navigate = useNavigate()
  useEffect(() => {
    if (!session) return
    const code = consumePendingJoin()
    if (code) {
      navigate({ path: `./join/${code}`, replace: true })
    }
  }, [session, navigate])

  const { peerId: peerIdRaw } = useSessionInfo(session)
  const peerId = peerIdRaw || null

  const path = usePath()
  const parentPaths = useParentPaths()
  const currentLevelPath = parentPaths[parentPaths.length - 1] ?? path
  const accountButtonKey = peerId ?? '?'

  const stateNamespace = useStateNamespace(['session'])

  const sessionStateAccessor: StateAtomAccessor = useMemo(() => {
    if (!session)
      return {
        value: null,
        loading: true,
        error: null,
        retry: () => props.sessionResource.retry(),
      }
    return {
      value: (storeId: string, signal?: AbortSignal) =>
        session.accessStateAtom({ storeId }, signal),
      loading: false,
      error: null,
      retry: () => {},
    }
  }, [session, props.sessionResource])

  const setOpenMenu = useBottomBarSetOpenMenu()

  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])

  const spacewaveSessionResource = useMemo<Resource<Session>>(
    () =>
      props.metadata?.providerId === 'spacewave'
        ? props.sessionResource
        : {
            value: null,
            loading: props.sessionResource.loading,
            error: props.sessionResource.error,
            retry: props.sessionResource.retry,
          },
    [props.metadata?.providerId, props.sessionResource],
  )

  const onboardingState = useStreamingResource(
    spacewaveSessionResource,
    (session, signal) => session.spacewave.watchOnboardingStatus(signal),
    [],
  )
  const lockStateResource = useStreamingResource(
    props.sessionResource,
    (session, signal) => session.watchLockState({}, signal),
    [],
  )
  const lockState = lockStateResource.value
  const isMountedPinLocked =
    lockState?.mode === SessionLockMode.PIN_ENCRYPTED &&
    (lockState?.locked ?? false)

  // Account status from Onboarding Status, the reactive cloud route-status
  // projection. Do not read stale session metadata for overlay routing.
  const accountStatus = onboardingState.value?.accountStatus
  const isDeleted =
    accountStatus === ProviderAccountStatus.ProviderAccountStatus_DELETED
  const isUnauthenticated =
    accountStatus ===
    ProviderAccountStatus.ProviderAccountStatus_UNAUTHENTICATED
  const isDormant =
    accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT

  const rootResource = useRootResource()
  const sessionIdx = useSessionIndex()
  const deletedRemovalStarted = useRef(false)
  const accountLabel =
    props.metadata?.displayName ||
    props.metadata?.cloudEntityId ||
    (sessionIdx != null ? `Session ${sessionIdx}` : null) ||
    peerId?.slice(-8) ||
    '?'

  const handleRemoveSession = useCallback(async () => {
    const root = rootResource.value
    if (!root || !sessionIdx) return
    await root.deleteSession(sessionIdx)
  }, [rootResource.value, sessionIdx])

  const handleReauth = useCallback(
    async (request: ReauthenticateSessionRequest) => {
      const root = rootResource.value
      if (!root) return
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      await sw.reauthenticateSession(request)
    },
    [rootResource.value],
  )
  const handleMountedUnlock = useCallback(
    async (pin: Uint8Array) => {
      if (!session) return
      await session.unlockSession(pin)
    },
    [session],
  )
  const handleReset = useCallback(
    async (idx: number, credential: EntityCredential) => {
      const root = rootResource.value
      if (!root) return
      await root.resetSession(idx, credential)
    },
    [rootResource.value],
  )

  useEffect(() => {
    if (!isDeleted || deletedRemovalStarted.current) return
    if (!rootResource.value || !sessionIdx) return
    deletedRemovalStarted.current = true
    void handleRemoveSession()
      .then(() => {
        navigate({ path: '/sessions', replace: true })
      })
      .catch(() => {
        deletedRemovalStarted.current = false
      })
  }, [handleRemoveSession, isDeleted, navigate, rootResource.value, sessionIdx])

  const isCloudProvider = props.metadata?.providerId === 'spacewave'

  const badgeLabel = isCloudProvider
    ? isDormant
      ? 'INACTIVE'
      : 'CLOUD'
    : 'LOCAL'
  const badgeClass = isDormant
    ? 'bg-warning/15 text-warning'
    : isCloudProvider
      ? 'bg-brand/15 text-brand'
      : 'bg-foreground/10 text-foreground-alt/70'

  const accountButton = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        className={className}
        data-testid="session-account-menu-button"
        aria-label={selected ? 'Close account menu' : 'Open account menu'}
      >
        {selected ? (
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        ) : (
          <LuPersonStanding {...bottomBarIconProps} aria-hidden="true" />
        )}
        <div className="max-w-36 truncate">{accountLabel}</div>
        {props.metadata && (
          <span
            data-testid="session-account-provider-badge"
            className={cn(
              'ml-1.5 rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase',
              badgeClass,
            )}
          >
            {badgeLabel}
          </span>
        )}
      </BottomBarItem>
    ),
    [accountLabel, badgeLabel, badgeClass, props.metadata],
  )

  const handleChangeAccount = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  // TODO: wire add auth method button in AuthMethodsSection

  const handleAccountBreadcrumb = useCallback(() => {
    navigate({ path: currentLevelPath })
  }, [navigate, currentLevelPath])

  const handleGoHome = useCallback(() => {
    navigate({ path: currentLevelPath, replace: true })
  }, [navigate, currentLevelPath])

  // Show full-page overlays for deleted or unauthenticated accounts.
  if (isDeleted && props.metadata) {
    return (
      <DeletedAccountOverlay
        metadata={props.metadata}
        onRemove={handleRemoveSession}
      />
    )
  }
  if (isUnauthenticated && props.metadata) {
    return (
      <ReAuthOverlay
        metadata={props.metadata}
        onReauth={handleReauth}
        onLogout={handleRemoveSession}
      />
    )
  }
  // Dormant sessions get the DormantOverlay gate, except on the /plan/
  // subtree so the user can actually reach UpgradeRouter to reactivate.
  // SpacewaveRootRouter also redirects the root route into /plan/upgrade
  // when dormant so bookmarked or direct entries converge on the same path.
  if (isDormant && props.metadata && !path.startsWith('/plan/')) {
    return <DormantOverlay metadata={props.metadata} />
  }
  if (isMountedPinLocked && props.metadata && sessionIdx != null) {
    return (
      <PinUnlockOverlay
        metadata={props.metadata}
        onUnlock={handleMountedUnlock}
        onReset={handleReset}
      />
    )
  }

  return (
    <SessionContext.Provider resource={props.sessionResource}>
      <SessionCommands />
      {session ? <RecoveryStatusPublisher session={session} /> : null}
      <StateNamespaceProvider
        stateNamespace={stateNamespace}
        stateAtomAccessor={sessionStateAccessor}
      >
        <DebugInfo>Session path: {path}</DebugInfo>
        <SessionUploadManagerProvider>
          <BottomBarLevel
            id="account"
            button={accountButton}
            overlay={
              <SessionDetails
                onCloseClick={handleCloseDetails}
                onChangeAccountClick={handleChangeAccount}
              />
            }
            buttonKey={accountButtonKey}
            menuLabel={accountLabel}
            onBreadcrumbClick={handleAccountBreadcrumb}
          >
            <SessionSyncStatusProvider>
              <SessionSelfEnrollmentStatusScope enabled={isCloudProvider}>
                <SessionSelfEnrollmentStatusButton />
                <SessionSyncStatusButton />
                <SystemStatusButton />
                <SessionUploadIndicator />
                <SessionProviderContainer
                  metadata={props.metadata}
                  spacewaveOnboarding={onboardingState.value ?? null}
                >
                  <TargetedInvitationInbox
                    sessionResource={spacewaveSessionResource}
                  />
                  <Routes>
                    {spacewaveSessionRoutes(props.metadata, {
                      fallbackPath: currentLevelPath,
                    })}
                    <Route path="/settings/cli/terminal">
                      <CliTerminalPage />
                    </Route>
                    <Route path="/settings/cli">
                      <CommandLineSetupPage />
                    </Route>
                    <Route path="/settings/transfer">
                      <TransferWizard />
                    </Route>
                    <Route path="/join/:code">
                      <JoinSpacePage />
                    </Route>
                    <Route path="/join">
                      <JoinSpacePage />
                    </Route>
                    <Route path="/pair">
                      <PairCodePage
                        session={session}
                        backPath="../"
                        donePath="../"
                      />
                    </Route>
                    <Route path="/setup/link-device">
                      <SessionFlowFrame fallbackPath={currentLevelPath}>
                        <LinkDeviceWizard />
                      </SessionFlowFrame>
                    </Route>
                    <Route path="/setup/provider">
                      <SessionFlowFrame fallbackPath={currentLevelPath}>
                        <ProviderSetup />
                      </SessionFlowFrame>
                    </Route>
                    <Route path="/setup/free-local">
                      <SessionFlowFrame fallbackPath={currentLevelPath}>
                        <LocalSessionSetup
                          mode="local"
                          metadata={props.metadata}
                        />
                      </SessionFlowFrame>
                    </Route>
                    <Route path="/setup/*">
                      <SessionFlowFrame fallbackPath={currentLevelPath}>
                        <SetupWizard />
                      </SessionFlowFrame>
                    </Route>
                    <Route path="/setup">
                      <SessionFlowFrame fallbackPath={currentLevelPath}>
                        <SetupWizard />
                      </SessionFlowFrame>
                    </Route>
                    <Route path="/">
                      <SessionRootRouter metadata={props.metadata} />
                    </Route>
                    <Route path="/billing/:baId/cancel">
                      <BillingCancelRoute />
                    </Route>
                    <Route path="/billing/:baId">
                      <BillingAccountDetailRoute />
                    </Route>
                    <Route path="/billing">
                      <BillingAccountsRoute />
                    </Route>
                    <Route path="/org/:orgId/new/:quickstartId">
                      <CreateSpaceRoute />
                    </Route>
                    <Route path="/org/:orgId/*">
                      <OrgContainer />
                    </Route>
                    <Route path="/new/:quickstartId">
                      <CreateSpaceRoute />
                    </Route>
                    <Route path="/so/:sharedObjectId/*">
                      <SessionSharedObjectContainer />
                    </Route>
                    <Route path="/so">
                      <Redirect to="../" />
                    </Route>
                    <Route path="*">
                      <div className="flex h-full w-full items-center justify-center px-4 py-8">
                        <div className="border-foreground/6 bg-background-card/30 flex w-full max-w-md flex-col items-center gap-3 rounded-lg border p-6 backdrop-blur-sm">
                          <div className="bg-foreground/5 flex size-10 items-center justify-center rounded-full">
                            <LuCompass
                              className="text-foreground-alt/60 size-5"
                              aria-hidden="true"
                            />
                          </div>
                          <h2 className="text-foreground text-sm font-semibold tracking-tight select-none">
                            Page not found
                          </h2>
                          <p className="text-foreground-alt/60 text-center text-xs">
                            No page exists at{' '}
                            <code className="text-foreground-alt/80 bg-foreground/5 rounded px-1.5 py-0.5 font-mono text-[0.7rem]">
                              {path}
                            </code>
                          </p>
                          <DashboardButton
                            icon={<LuArrowLeft className="size-3.5" />}
                            onClick={handleGoHome}
                          >
                            Back to dashboard
                          </DashboardButton>
                        </div>
                      </div>
                    </Route>
                  </Routes>
                </SessionProviderContainer>
              </SessionSelfEnrollmentStatusScope>
            </SessionSyncStatusProvider>
          </BottomBarLevel>
        </SessionUploadManagerProvider>
      </StateNamespaceProvider>
    </SessionContext.Provider>
  )
}

function SessionSelfEnrollmentStatusScope({
  enabled,
  children,
}: {
  enabled: boolean
  children: ReactNode
}) {
  if (!enabled) return <>{children}</>

  return (
    <SessionSelfEnrollmentStatusProvider>
      {children}
    </SessionSelfEnrollmentStatusProvider>
  )
}

function TargetedInvitationInbox(props: {
  sessionResource: Resource<Session>
}) {
  const inbox = useStreamingResource(
    props.sessionResource,
    (session, signal) => session.spacewave.watchTargetedInvitations(signal),
    [],
  )
  const session = props.sessionResource.value
  const [processing, setProcessing] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const pending = useMemo(
    () =>
      (inbox.value?.invitations ?? []).filter(
        (inv) => inv.status === 'pending',
      ),
    [inbox.value?.invitations],
  )

  const handleAccept = useCallback(
    async (inv: TargetedInvitationInfo) => {
      if (!session || processing) return
      setProcessing(inv.id ?? '')
      setError(null)
      try {
        if (inv.purpose === TargetedInvitePurpose.SPACE) {
          await session.spacewave.acceptSpaceTargetedInvitation(inv.id ?? '')
          return
        }
        if (inv.purpose === TargetedInvitePurpose.ORGANIZATION) {
          await session.spacewave.acceptOrganizationTargetedInvitation(
            inv.id ?? '',
          )
          return
        }
        await session.spacewave.processTargetedInvitation(
          inv.id ?? '',
          'accept',
        )
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to accept invite')
      } finally {
        setProcessing(null)
      }
    },
    [processing, session],
  )

  const handleDecline = useCallback(
    async (inv: TargetedInvitationInfo) => {
      if (!session || processing) return
      setProcessing(inv.id ?? '')
      setError(null)
      try {
        await session.spacewave.processTargetedInvitation(
          inv.id ?? '',
          'decline',
        )
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to decline invite',
        )
      } finally {
        setProcessing(null)
      }
    },
    [processing, session],
  )

  if (!session || pending.length === 0) return null

  return (
    <div className="pointer-events-none fixed right-4 bottom-16 z-40 w-[min(22rem,calc(100vw-2rem))]">
      <div className="border-foreground/10 bg-background-card/95 pointer-events-auto rounded-lg border p-3 shadow-lg backdrop-blur">
        <div className="mb-2 flex items-center gap-2">
          <LuInbox className="text-foreground-alt size-4" />
          <div className="text-foreground text-sm font-medium">
            Pending Invites
          </div>
        </div>
        <div className="space-y-2">
          {pending.slice(0, 3).map((inv) => (
            <div
              key={inv.id}
              className="border-foreground/10 rounded-md border p-2"
            >
              <div className="text-foreground truncate text-xs font-medium">
                {targetedInvitationTitle(inv)}
              </div>
              <div className="text-foreground-alt/60 truncate text-[11px]">
                {inv.role || 'reader'} from {inv.actorAccountId}
              </div>
              <div className="mt-2 flex gap-2">
                <button
                  type="button"
                  disabled={!!processing}
                  onClick={() => void handleAccept(inv)}
                  className={cn(
                    'flex flex-1 items-center justify-center gap-1.5 rounded-md border px-2 py-1.5 text-xs transition-colors',
                    'border-green-500/30 text-green-500 hover:bg-green-500/10',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  <LuCheck className="size-3.5" />
                  Accept
                </button>
                <button
                  type="button"
                  disabled={!!processing}
                  onClick={() => void handleDecline(inv)}
                  className={cn(
                    'flex flex-1 items-center justify-center gap-1.5 rounded-md border px-2 py-1.5 text-xs transition-colors',
                    'border-foreground/15 text-foreground-alt hover:bg-foreground/5',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  <LuX className="size-3.5" />
                  Decline
                </button>
              </div>
            </div>
          ))}
        </div>
        {error && <div className="text-destructive mt-2 text-xs">{error}</div>}
      </div>
    </div>
  )
}

function targetedInvitationTitle(inv: TargetedInvitationInfo): string {
  if (inv.purpose === TargetedInvitePurpose.SPACE) {
    return `Space invite: ${inv.contextId || 'unknown space'}`
  }
  if (inv.purpose === TargetedInvitePurpose.ORGANIZATION) {
    return `Organization invite: ${inv.contextId || 'unknown org'}`
  }
  return 'Invitation'
}
