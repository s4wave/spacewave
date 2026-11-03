import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
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
} from '@s4wave/web/images/icons.js'
import { type Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { DebugInfo } from '@aptre/bldr-react'

import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

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
import { InvitePage } from '@s4wave/app/provider/spacewave/InvitePage.js'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { consumePendingRoute } from '@s4wave/app/routes/pendingRoute.js'
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

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '9px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    marginLeft: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 36)`,
  },
  rule2: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule4: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule7: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: '.7rem',
    lineHeight: 1.5,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule8: {
    pointerEvents: 'none',
    position: 'fixed',
    zIndex: 40,
    bottom: `calc(${themeLayout['--spacing']} * 16)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    width: themeLayout['--width-session-overlay'],
  },
  rule9: {
    '--tw-backdrop-blur': 'blur(8px)',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    pointerEvents: 'auto',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule14: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-green-500']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-green-500'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule17: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground-alt'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule18: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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

  // Open a route SessionRedirect stored before a session existed.
  useEffect(() => {
    if (!session) return
    const route = consumePendingRoute(environment.documentStorage)
    if (route) {
      navigate({ path: `./${route}`, replace: true })
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
    sx?: BottomBarStyles,
  ) => ReactNode
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
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        sx={sx}
        data-testid="session-account-menu-button"
        aria-label={selected ? 'Close account menu' : 'Open account menu'}
      >
        {selected ? (
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        ) : (
          <LuPersonStanding {...bottomBarIconProps} aria-hidden="true" />
        )}
        <div sx={stylexStyles2.rule1}>{label}</div>
        {metadata && (
          <span
            data-testid="session-account-provider-badge"
            sx={[[stylexStyles4.rule1], [badge.sx]]}
          >
            {badge.label}
          </span>
        )}
      </BottomBarItem>
    ),
    [label, badge.label, badge.sx, metadata],
  )

  return { button, label }
}

// accountBadge names the account's provider, or marks a dormant cloud account.
function accountBadge(isCloudProvider: boolean, isDormant: boolean) {
  if (isDormant) {
    return { label: 'INACTIVE', sx: stylexStyles3.rule1 }
  }
  if (isCloudProvider) {
    return { label: 'CLOUD', sx: stylexStyles3.rule2 }
  }
  return {
    label: 'LOCAL',
    sx: stylexStyles3.rule3,
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
              <SessionSelfEnrollmentStatusScope enabled={isCloudProvider}>
                <SessionProviderContainer
                  providerId={providerId}
                  metadata={props.metadata}
                  spacewaveOnboarding={onboardingState.value ?? null}
                >
                  <BottomBarLevel
                    id="account"
                    button={accountButton.button}
                    overlay={
                      <SessionDetails
                        onCloseClick={handleCloseDetails}
                        onChangeAccountClick={handleChangeAccount}
                      />
                    }
                    menuLabel={accountButton.label}
                    onBreadcrumbClick={handleAccountBreadcrumb}
                  >
                    <SessionSelfEnrollmentStatusButton />
                    <SessionBackgroundPluginsButton />
                    <SessionSyncStatusButton />
                    <AgentConnectButton />
                    <SystemStatusButton />
                    <SessionUploadIndicator />
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
                      <Route path="/invite/:code">
                        <InvitePage />
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
                        <div sx={stylexStyles2.rule2}>
                          <div sx={stylexStyles2.rule3}>
                            <div sx={stylexStyles2.rule4}>
                              <LuCompass
                                sx={stylexStyles.rule1}
                                aria-hidden="true"
                              />
                            </div>
                            <h2 sx={stylexStyles2.rule5}>Page not found</h2>
                            <p sx={stylexStyles2.rule6}>
                              No page exists at{' '}
                              <code sx={stylexStyles2.rule7}>{path}</code>
                            </p>
                            <DashboardButton
                              icon={<LuArrowLeft sx={stylexStyles.rule2} />}
                              onClick={handleGoHome}
                            >
                              Back to dashboard
                            </DashboardButton>
                          </div>
                        </div>
                      </Route>
                    </Routes>
                  </BottomBarLevel>
                </SessionProviderContainer>
              </SessionSelfEnrollmentStatusScope>
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
    <div sx={stylexStyles2.rule8}>
      <div sx={stylexStyles2.rule9}>
        <div sx={stylexStyles2.rule10}>
          <LuInbox sx={stylexStyles.rule3} />
          <div sx={stylexStyles2.rule11}>Pending Invites</div>
        </div>
        <div>
          {pending.slice(0, 3).map((inv) => (
            <div
              key={inv.id}
              sx={[nativeChildSpacing.s2, stylexStyles2.rule12]}
            >
              <div sx={stylexStyles2.rule13}>
                {targetedInvitationTitle(inv)}
              </div>
              <div sx={stylexStyles2.rule14}>
                {inv.role || 'reader'} from {inv.actorAccountId}
              </div>
              <div sx={stylexStyles2.rule15}>
                <button
                  type="button"
                  disabled={!!processing}
                  onClick={() => void handleAccept(inv)}
                  sx={[stylexStyles2.rule16]}
                >
                  <LuCheck sx={stylexStyles.rule2} />
                  Accept
                </button>
                <button
                  type="button"
                  disabled={!!processing}
                  onClick={() => void handleDecline(inv)}
                  sx={[stylexStyles2.rule17]}
                >
                  <LuX sx={stylexStyles.rule2} />
                  Decline
                </button>
              </div>
            </div>
          ))}
        </div>
        {error && <div sx={stylexStyles2.rule18}>{error}</div>}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
