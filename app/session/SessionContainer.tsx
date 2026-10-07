import {
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  LuArrowLeft,
  LuArrowUp,
  LuCompass,
  LuCheck,
  LuInbox,
  LuPersonStanding,
  LuX,
} from 'react-icons/lu'
import { type Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { DebugInfo } from '@aptre/bldr-react'

import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import {
  Route,
  Routes,
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { BillingAccountDetailRoute } from '@s4wave/app/billing/BillingAccountDetailRoute.js'
import { BillingAccountsRoute } from '@s4wave/app/billing/BillingAccountsRoute.js'
import { BillingCancelRoute } from '@s4wave/app/billing/BillingCancelRoute.js'
import { OrgContainer } from '@s4wave/app/org/OrgContainer.js'
import { JoinSpacePage } from '@s4wave/app/sobject/JoinSpacePage.js'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { consumePendingJoin } from '@s4wave/app/routes/pendingJoin.js'
import { SessionPageRoutes } from '@s4wave/app/routes/SessionPageRoutes.js'
import { CreateSpaceRoute } from '@s4wave/app/quickstart/CreateSpaceRoute.js'
import { SpacewaveRootRouter } from '@s4wave/app/provider/spacewave/SpacewaveRootRouter.js'
import { spacewaveSessionRoutes } from '@s4wave/app/provider/spacewave/SpacewaveSessionRoutes.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { bottomBarIconProps } from '@s4wave/web/frame/bottom-icon-props.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  StateNamespaceProvider,
  useStateNamespace,
  type StateAtomAccessor,
} from '@s4wave/web/state/index.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import type { ReauthenticateSessionRequest } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import {
  SessionLockMode,
  type EntityCredential,
} from '@s4wave/core/session/session.pb.js'
import { SystemStatusButton } from '@s4wave/app/system/SystemStatusButton.js'
import { AgentConnectButton } from './agent/AgentConnectButton.js'
import { RecoveryStatusPublisher } from '@s4wave/app/system/RecoveryStatusPublisher.js'
import {
  TargetedInvitePurpose,
  type TargetedInvitationInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { SessionDashboardContainer } from './SessionDashboardContainer.js'
import { SessionSharedObjectContainer } from './SessionSharedObjectContainer.js'
import { SetupWizard } from './SetupWizard.js'
import { ProviderSetup } from './setup/ProviderSetup.js'
import { LocalSessionSetup } from './setup/LocalSessionSetup.js'
import { CommandLineSetupPage } from './settings/CommandLineSetupPage.js'
import { PluginRepositoriesPage } from './settings/PluginRepositoriesPage.js'
import { CliTerminalPage } from './settings/CliTerminalPage.js'
import { TransferWizard } from './settings/TransferWizard.js'
import { StorageHealthPage } from './storage/StorageHealthPage.js'
import { SessionProviderContainer } from './SessionProviderContainer.js'
import { SessionCommands } from './SessionCommands.js'
import { SessionDetails } from './dashboard/SessionDetails.js'
import { DeletedAccountOverlay } from './DeletedAccountOverlay.js'
import { DormantOverlay } from './DormantOverlay.js'
import { ReAuthOverlay } from './ReAuthOverlay.js'
import { SessionFlowFrame } from './SessionFlowFrame.js'
import { PinUnlockOverlay } from './PinUnlockOverlay.js'
import { SessionSyncStatusButton } from './SessionSyncStatusButton.js'
import { SessionSyncStatusProvider } from './SessionSyncStatusContext.js'
import { SessionStorageStatsProvider } from './SessionStorageStatsContext.js'
import { SessionBackgroundPluginsButton } from './SessionBackgroundPluginsButton.js'
import { SessionSelfEnrollmentStatusButton } from './SessionSelfEnrollmentStatusButton.js'
import { SessionSelfEnrollmentStatusProvider } from './SessionSelfEnrollmentStatusContext.js'
import {
  SessionUploadManagerProvider,
  SessionUploadIndicator,
} from './SessionUploadManagerContext.js'

const LazyPairCodePage = lazy(async () => {
  const { PairCodePage } = await import('@s4wave/app/pair/PairCodePage.js')
  return { default: PairCodePage }
})

const LazyLinkDeviceWizard = lazy(async () => {
  const { LinkDeviceWizard } = await import('./setup/LinkDeviceWizard.js')
  return { default: LinkDeviceWizard }
})

// SessionRootRouter handles the root route redirect logic for a session.
// Dispatches to SpacewaveRootRouter for cloud sessions.
// Local sessions show the dashboard directly.
function SessionRootRouter(props: { providerId?: string }) {
  if (props.providerId === 'spacewave') {
    return <SpacewaveRootRouter />
  }

  return <SessionDashboardContainer />
}

interface SessionContainerProps {
  sessionResource: Resource<Session>
  metadata?: SessionMetadata
}

// useSessionArrival marks that this browser has product state to return to and
// opens a join code stashed before the Session mounted.
function useSessionArrival(session: Session | null) {
  const environment = useAppEnvironment()
  const navigate = useNavigate()

  // Return visitors with the flag see the loading screen instead of landing.
  useEffect(() => {
    if (session && !environment.storage.getItem('spacewave-has-session')) {
      environment.storage.setItem('spacewave-has-session', '1')
    }
  }, [session, environment.storage])

  // Pick up a pending join code stashed by JoinRedirect (no-session path).
  useEffect(() => {
    if (!session) return
    const code = consumePendingJoin(environment.documentStorage)
    if (code) {
      navigate({ path: `./join/${code}`, replace: true })
    }
  }, [session, navigate, environment.documentStorage])
}

// useSessionStateAccessor reads the Session's state atoms once it mounts.
function useSessionStateAccessor(
  sessionResource: Resource<Session>,
): StateAtomAccessor {
  const session = sessionResource.value
  return useMemo(() => {
    if (!session) {
      return {
        value: null,
        loading: true,
        error: null,
        retry: () => sessionResource.retry(),
      }
    }
    return {
      value: (storeId: string, signal?: AbortSignal) =>
        session.accessStateAtom({ storeId }, signal),
      loading: false,
      error: null,
      retry: () => {},
    }
  }, [session, sessionResource])
}

// useSessionAccountOverlay returns the full-page overlay that replaces the
// Session for a deleted, unauthenticated, dormant, or locked account, or null.
// It removes a deleted account's Session once the root resource is ready.
function useSessionAccountOverlay(
  props: SessionContainerProps,
  accountStatus: ProviderAccountStatus | undefined,
  path: string,
): ReactNode {
  const session = props.sessionResource.value
  const metadata = props.metadata
  const navigate = useNavigate()
  const root = useRootResource().value
  const sessionIdx = useSessionIndex()
  const deletedRemovalStarted = useRef(false)

  const lockState = useStreamingResource(
    props.sessionResource,
    (session, signal) => session.watchLockState({}, signal),
    [],
  ).value
  const isLocked =
    lockState?.mode === SessionLockMode.PIN_ENCRYPTED && !!lockState.locked
  const isDeleted =
    accountStatus === ProviderAccountStatus.ProviderAccountStatus_DELETED

  const handleRemove = useCallback(async () => {
    if (!root || !sessionIdx) return
    await root.deleteSession(sessionIdx)
  }, [root, sessionIdx])

  const handleReauth = useCallback(
    async (request: ReauthenticateSessionRequest) => {
      if (!root) return
      using provider = await root.lookupProvider('spacewave')
      const sw = new SpacewaveProvider(provider.resourceRef)
      await sw.reauthenticateSession(request)
    },
    [root],
  )

  const handleUnlock = useCallback(
    async (pin: Uint8Array) => {
      await session?.unlockSession(pin)
    },
    [session],
  )

  const handleReset = useCallback(
    async (idx: number, credential: EntityCredential) => {
      await root?.resetSession(idx, credential)
    },
    [root],
  )

  // Remove a deleted account's Session once, retrying after a failure.
  useEffect(() => {
    if (!isDeleted || deletedRemovalStarted.current) return
    if (!root || !sessionIdx) return
    deletedRemovalStarted.current = true
    void handleRemove()
      .then(() => {
        navigate({ path: '/sessions', replace: true })
      })
      .catch(() => {
        deletedRemovalStarted.current = false
      })
  }, [handleRemove, isDeleted, navigate, root, sessionIdx])

  if (!metadata) return null
  switch (accountStatus) {
    case ProviderAccountStatus.ProviderAccountStatus_DELETED:
      return (
        <DeletedAccountOverlay metadata={metadata} onRemove={handleRemove} />
      )
    case ProviderAccountStatus.ProviderAccountStatus_UNAUTHENTICATED:
      return (
        <ReAuthOverlay
          metadata={metadata}
          onReauth={handleReauth}
          onLogout={handleRemove}
        />
      )
    case ProviderAccountStatus.ProviderAccountStatus_DORMANT:
      // The /plan/ subtree stays reachable so the user can reactivate.
      // SpacewaveRootRouter also redirects the root route into /plan/upgrade.
      if (!path.startsWith('/plan/')) {
        return <DormantOverlay metadata={metadata} />
      }
  }
  if (isLocked && sessionIdx != null) {
    return (
      <PinUnlockOverlay
        metadata={metadata}
        onUnlock={handleUnlock}
        onReset={handleReset}
      />
    )
  }
  return null
}

// AccountButton is the account bottom-bar item and the key that refreshes it.
interface AccountButton {
  button: (
    selected: boolean,
    onClick: () => void,
    className?: string,
  ) => ReactNode
  buttonKey: string
  label: string
}

// useAccountButton renders the account bottom-bar item with the account label
// and its provider badge.
function useAccountButton(
  props: SessionContainerProps,
  isCloudProvider: boolean,
  isDormant: boolean,
): AccountButton {
  const metadata = props.metadata
  const sessionIdx = useSessionIndex()
  const peerId = useSessionInfo(props.sessionResource.value).peerId || null
  const label =
    metadata?.displayName ||
    metadata?.cloudEntityId ||
    (sessionIdx != null ? `Session ${sessionIdx}` : null) ||
    peerId?.slice(-8) ||
    '?'

  const badge = accountBadge(isCloudProvider, isDormant)
  const button = useCallback(
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
        <div className="max-w-36 truncate">{label}</div>
        {metadata && (
          <span
            data-testid="session-account-provider-badge"
            className={cn(
              'ml-1.5 rounded-full px-1.5 py-0.5 micro-nine font-semibold tracking-wider uppercase',
              badge.className,
            )}
          >
            {badge.label}
          </span>
        )}
      </BottomBarItem>
    ),
    [label, badge.label, badge.className, metadata],
  )

  // Refresh the registered button when metadata arrives or its badge changes.
  const buttonKey = `${peerId ?? '?'}/${metadata ? badge.label : ''}`
  return { button, buttonKey, label }
}

// accountBadge names the account's provider, or marks a dormant cloud account.
function accountBadge(isCloudProvider: boolean, isDormant: boolean) {
  if (isDormant) {
    return { label: 'INACTIVE', className: 'bg-warning/15 text-warning' }
  }
  if (isCloudProvider) {
    return { label: 'CLOUD', className: 'bg-brand/15 text-brand' }
  }
  return {
    label: 'LOCAL',
    className: 'bg-foreground/10 text-foreground-alt/70',
  }
}

// SessionContainer is the top-level URL router for a Session. Nested routes
// register their own items under the account bottom-bar level.
export function SessionContainer(props: SessionContainerProps) {
  const session = props.sessionResource.value
  const providerId =
    session?.sessionRef?.providerResourceRef?.providerId ??
    props.metadata?.providerId
  const isCloudProvider = providerId === 'spacewave'
  useSessionArrival(session)

  // Only cloud Sessions watch Onboarding Status, the reactive account status.
  // Do not read stale session metadata for overlay routing.
  const spacewaveSessionResource = useMemo<Resource<Session>>(
    () =>
      isCloudProvider
        ? props.sessionResource
        : { ...props.sessionResource, value: null },
    [isCloudProvider, props.sessionResource],
  )
  const onboardingState = useStreamingResource(
    spacewaveSessionResource,
    (session, signal) => session.spacewave.watchOnboardingStatus(signal),
    [],
  )
  const accountStatus = onboardingState.value?.accountStatus
  const isDormant =
    accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT

  const path = usePath()
  const parentPaths = useParentPaths()
  const currentLevelPath = parentPaths[parentPaths.length - 1] ?? path
  const overlay = useSessionAccountOverlay(props, accountStatus, path)
  const accountButton = useAccountButton(props, isCloudProvider, isDormant)
  const stateNamespace = useStateNamespace(['session'])
  const sessionStateAccessor = useSessionStateAccessor(props.sessionResource)

  // Navigate from the account menu and the not-found page.
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])
  const handleChangeAccount = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])
  const handleAccountBreadcrumb = useCallback(() => {
    navigate({ path: currentLevelPath })
  }, [navigate, currentLevelPath])
  const handleGoHome = useCallback(() => {
    navigate({ path: currentLevelPath, replace: true })
  }, [navigate, currentLevelPath])

  if (overlay) return overlay

  return (
    <SessionContext.Provider resource={props.sessionResource}>
      <SessionCommands />
      {session ? <RecoveryStatusPublisher session={session} /> : null}
      <StateNamespaceProvider
        stateNamespace={stateNamespace}
        stateAtomAccessor={sessionStateAccessor}
      >
        <DebugInfo>Session path: {path}</DebugInfo>
        <SessionStorageStatsProvider>
          <SessionSyncStatusProvider>
            <SessionUploadManagerProvider>
              <BottomBarLevel
                id="account"
                button={accountButton.button}
                overlay={
                  <SessionDetails
                    onCloseClick={handleCloseDetails}
                    onChangeAccountClick={handleChangeAccount}
                  />
                }
                buttonKey={accountButton.buttonKey}
                menuLabel={accountButton.label}
                onBreadcrumbClick={handleAccountBreadcrumb}
              >
                <SessionSelfEnrollmentStatusScope enabled={isCloudProvider}>
                  <SessionSelfEnrollmentStatusButton />
                  <SessionBackgroundPluginsButton />
                  <SessionSyncStatusButton />
                  <AgentConnectButton />
                  <SystemStatusButton />
                  <SessionUploadIndicator />
                  <SessionProviderContainer
                    providerId={providerId}
                    metadata={props.metadata}
                    spacewaveOnboarding={onboardingState.value ?? null}
                  >
                    <TargetedInvitationInbox
                      sessionResource={spacewaveSessionResource}
                    />
                    <Routes>
                      {SessionPageRoutes}
                      <Route path="/settings/storage/recovery/incident">
                        <StorageHealthPage recovery storageIncident />
                      </Route>
                      <Route path="/settings/storage/recovery">
                        <StorageHealthPage recovery />
                      </Route>
                      <Route path="/settings/storage">
                        <StorageHealthPage />
                      </Route>
                      {spacewaveSessionRoutes(props.metadata, {
                        fallbackPath: currentLevelPath,
                      })}
                      <Route path="/settings/cli/terminal">
                        <CliTerminalPage />
                      </Route>
                      <Route path="/settings/cli">
                        <CommandLineSetupPage />
                      </Route>
                      <Route path="/settings/plugins">
                        <PluginRepositoriesPage />
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
                        <LazyPairCodePage
                          session={session}
                          backPath="../"
                          donePath="../"
                        />
                      </Route>
                      <Route path="/setup/link-device">
                        <SessionFlowFrame fallbackPath={currentLevelPath}>
                          <LazyLinkDeviceWizard />
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
                        <SessionRootRouter providerId={providerId} />
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
                      <Route path="/new/:quickstartId/on/:storage">
                        <CreateSpaceRoute />
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
                              <code className="text-foreground-alt/80 bg-foreground/5 micro-seven rounded px-1.5 py-0.5 font-mono">
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
              </BottomBarLevel>
            </SessionUploadManagerProvider>
          </SessionSyncStatusProvider>
        </SessionStorageStatsProvider>
      </StateNamespaceProvider>
    </SessionContext.Provider>
  )
}

// SessionSelfEnrollmentStatusScope provides self-enrollment status to cloud
// Sessions only.
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

// TargetedInvitationInbox lists pending targeted invitations from the watched
// inbox with Accept and Decline.
function TargetedInvitationInbox(props: {
  sessionResource: Resource<Session>
}) {
  const inbox = useStreamingResource(
    props.sessionResource,
    (session, signal) => session.spacewave.watchTargetedInvitations(signal),
    [],
  )
  const session = props.sessionResource.value
  const navigate = useNavigate()
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
          const response =
            await session.spacewave.acceptSpaceTargetedInvitation(inv.id ?? '')
          if (response.joinResult === 'accepted' && response.sharedObjectId) {
            navigate({
              path: `so/${encodeURIComponent(response.sharedObjectId)}`,
            })
          }
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
    [navigate, processing, session],
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
    <div className="pointer-events-none fixed right-4 bottom-16 z-40 w-(--width-session-overlay)">
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
              <div className="text-foreground-alt/60 truncate text-xs">
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

// targetedInvitationTitle names what a targeted invitation joins.
function targetedInvitationTitle(inv: TargetedInvitationInfo): string {
  if (inv.purpose === TargetedInvitePurpose.SPACE) {
    return `Space invite: ${inv.contextId || 'unknown space'}`
  }
  if (inv.purpose === TargetedInvitePurpose.ORGANIZATION) {
    return `Organization invite: ${inv.contextId || 'unknown org'}`
  }
  return 'Invitation'
}
