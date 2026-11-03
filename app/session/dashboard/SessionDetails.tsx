import { lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuChevronDown } from '@s4wave/web/images/icons.js'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { BillingSection } from '@s4wave/app/billing/BillingSection.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import {
  RootContext,
  SessionContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  resolvePath,
  Route,
  Router,
  Routes,
  type To,
  useNavigate,
} from '@s4wave/web/router/router.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'

import {
  WatchLocalDisplayNameRequest,
  WatchLocalDisplayNameResponse,
} from '@s4wave/sdk/session/local-session.pb.js'

import { AccountDashboardStateProvider } from './AccountDashboardStateContext.js'
import { AuthMethodsSection } from './AuthMethodsSection.js'
import { CryptoKeysSection } from './CryptoKeysSection.js'
import { DisplayNameCard } from './DisplayNameCard.js'
import { EmailSection } from './EmailSection.js'
import { IdentifiersSection } from './IdentifiersSection.js'
import { OrganizationsSection } from './OrganizationsSection.js'
import { SecuritySection } from './SecuritySection.js'
import { SessionActionsSection } from './SessionActionsSection.js'
import { SessionDetailsHeader } from './SessionDetailsHeader.js'
import { SessionsSection } from './SessionsSection.js'
import { SessionSyncStatusSummary } from './SessionSyncStatusSummary.js'
import { StorageHealthSection } from '../storage/StorageHealthSection.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'flex-end',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule4: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 90%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    '--tw-shadow':
      '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: { color: themeColors['--color-destructive'] },
  rule8: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const LazyLinkDeviceWizard = lazy(async () => {
  const { LinkDeviceWizard } = await import('../setup/LinkDeviceWizard.js')
  return { default: LinkDeviceWizard }
})

export interface SessionDetailsProps {
  onCloseClick?: () => void
  onChangeAccountClick?: () => void
}

type SessionOpenSection =
  | 'account'
  | 'auth-methods'
  | 'email'
  | 'security'
  | 'sessions'
  | 'orgs'
  | 'billing'
  | 'storage'
  | 'crypto'
  | 'identifiers'

// SectionProps binds a collapsible section to the shared open section.
type SectionProps = (section: SessionOpenSection) => {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// describeSessionHeader returns the title and subtitle of the details header.
function describeSessionHeader(
  currentDisplayName: string,
  providerDisplayName: string | undefined,
  providerId: string | undefined,
): { title: string; subtitle: string } {
  const providerLabel =
    providerId === 'local'
      ? 'Local session'
      : providerId === 'spacewave'
        ? 'Cloud account'
        : 'Session'
  return {
    title: currentDisplayName || 'Account settings',
    subtitle: providerDisplayName || providerLabel,
  }
}

// useLocalDisplayName watches the local provider's display name. It is
// undefined for cloud sessions or before the first value arrives.
function useLocalDisplayName(
  session: Session | null | undefined,
  isLocal: boolean,
): string | undefined {
  const request = useMemo<WatchLocalDisplayNameRequest>(() => ({}), [])
  const response = useWatchStateRpc(
    useCallback(
      (req: WatchLocalDisplayNameRequest, signal: AbortSignal) =>
        isLocal && session
          ? session.localProvider.watchDisplayName(req, signal)
          : null,
      [isLocal, session],
    ),
    request,
    WatchLocalDisplayNameRequest.equals,
    WatchLocalDisplayNameResponse.equals,
  )
  return response?.displayName
}

interface SessionLogoutOptions {
  account: Account | null | undefined
  peerId: string
  root: Root | null | undefined
  sessionIdx: number | null
}

// useSessionLogout revokes the cloud session, deletes the local session, and
// returns to the session list. It owns the confirmation dialog state.
function useSessionLogout({
  account,
  peerId,
  root,
  sessionIdx,
}: SessionLogoutOptions) {
  const navigate = useNavigate()
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutOpen, setLogoutOpen] = useState(false)

  const confirmLogout = async () => {
    if (!account || !peerId || peerId === 'Unknown') return
    setLoggingOut(true)
    await account.selfRevokeSession(peerId).catch(() => {})
    if (root && sessionIdx != null) {
      await root.deleteSession(sessionIdx).catch(() => {})
    }
    navigate({ path: '/sessions' })
  }

  return { loggingOut, logoutOpen, setLogoutOpen, confirmLogout }
}

// useSessionLock locks a PIN-protected session, or returns to the session
// list when the session has no PIN lock.
function useSessionLock(
  session: Session | null | undefined,
  sessionIdx: number | null,
  isPinMode: boolean,
) {
  const navigate = useNavigate()
  const [locking, setLocking] = useState(false)

  const lock = async () => {
    if (!isPinMode) {
      navigate({ path: '/sessions' })
      return
    }
    if (!session || sessionIdx == null) return
    setLocking(true)
    try {
      await session.lockSession()
    } catch {
      // Session transport may close during lock, but the lock still takes effect.
    }
    navigate({ path: '/sessions', replace: true })
  }

  return { locking, lock }
}

// useSettingsScrollCue shows a scroll cue while the settings viewport has
// content below the fold. It re-measures when the open section changes.
function useSettingsScrollCue(openSection: SessionOpenSection | null) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [showCue, setShowCue] = useState(false)

  const update = useCallback(() => {
    const viewport = scrollRef.current
    if (!viewport) return
    setShowCue(
      viewport.scrollTop + viewport.clientHeight < viewport.scrollHeight - 1,
    )
  }, [])

  useEffect(() => {
    update()
  }, [openSection, update])

  const scrollMore = () => {
    scrollRef.current?.scrollBy({ top: 160, behavior: 'smooth' })
  }

  return { scrollRef, showCue, update, scrollMore }
}

// SessionDetails displays account/session information and actions.
export function SessionDetails({
  onCloseClick,
  onChangeAccountClick,
}: SessionDetailsProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigateSession = useSessionNavigate()
  const sessionIdx = useSessionIndex() || null
  const ns = useStateNamespace(['session-settings'])
  const rootResource = RootContext.useContext()
  const root = useResourceValue(rootResource)
  const metadata = useSessionMetadata(sessionIdx)

  const lockStateResource = useStreamingResource(
    sessionResource,
    (session, signal) => session.watchLockState({}, signal),
    [],
  )
  const lockState = lockStateResource.value
  const isPinMode = lockState?.mode === SessionLockMode.PIN_ENCRYPTED

  const { sessionInfo, loading, error, providerId, accountId } =
    useSessionInfo(session)
  const accountResource = useMountAccount(providerId, accountId)
  const account = accountResource.value
  const peerId = sessionInfo?.peerId ?? 'Unknown'
  const isLocal = providerId === 'local'

  const [detailsPath, setDetailsPath] = useStateAtom<string>(
    ns,
    'details-path',
    '/',
  )
  const [dangerZoneOpen, setDangerZoneOpen] = useStateAtom<boolean>(
    ns,
    'danger-zone-open',
    false,
  )
  const [openSection, setOpenSection] = useStateAtom<SessionOpenSection | null>(
    ns,
    'open-section',
    isLocal ? 'account' : 'auth-methods',
  )
  const currentDisplayName =
    useLocalDisplayName(session, isLocal) ?? metadata?.displayName ?? ''
  const header = describeSessionHeader(
    currentDisplayName,
    metadata?.providerDisplayName,
    providerId,
  )

  // Only show transfer button when there are multiple sessions to transfer between.
  const sessionsResource = useSessionList()
  const showTransfer = (sessionsResource.value?.sessions?.length ?? 0) > 1

  const logout = useSessionLogout({ account, peerId, root, sessionIdx })
  const { locking, lock } = useSessionLock(session, sessionIdx, isPinMode)
  const scrollCue = useSettingsScrollCue(openSection)

  useEffect(() => {
    return () => {
      setDetailsPath('/')
    }
  }, [setDetailsPath])

  const handleCloseDetails = () => {
    setDetailsPath('/')
    onCloseClick?.()
  }

  const sectionProps: SectionProps = (section) => ({
    open: openSection === section,
    onOpenChange: (open) => setOpenSection(open ? section : null),
  })

  const handleSettingsNavigate = (path: string) => {
    handleCloseDetails()
    navigateSession({ path })
  }

  if (loading) {
    return <DetailsStatus>Loading session info…</DetailsStatus>
  }

  if (error) {
    return <DetailsStatus destructive>Error: {error.message}</DetailsStatus>
  }

  return (
    <Router
      path={detailsPath}
      onNavigate={(to: To) => setDetailsPath((curr) => resolvePath(curr, to))}
    >
      <Routes fullPath>
        <Route path="/link-device">
          <LazyLinkDeviceWizard exitPath="/" />
        </Route>
        <Route path="/">
          <div sx={stylexStyles2.rule1}>
            <SessionDetailsHeader
              title={header.title}
              subtitle={header.subtitle}
              peerId={peerId !== 'Unknown' ? peerId : ''}
              lockDisabled={!lockState || locking}
              showLogout={!isLocal}
              loggingOut={logout.loggingOut}
              onChangeAccount={() => {
                setDetailsPath('/')
                onChangeAccountClick?.()
              }}
              onLock={() => void lock()}
              onLogout={() => logout.setLogoutOpen(true)}
              onClose={onCloseClick && handleCloseDetails}
            />

            <div
              ref={scrollCue.scrollRef}
              sx={stylexStyles2.rule2}
              onScroll={scrollCue.update}
            >
              <div sx={false}>
                <SettingsSections
                  sx={nativeChildSpacing.s3}
                  isLocal={isLocal}
                  accountResource={accountResource}
                  session={session}
                  currentDisplayName={currentDisplayName}
                  sectionProps={sectionProps}
                  onNavigateToPath={handleSettingsNavigate}
                  onOpenOrganization={(orgId) => {
                    if (!orgId) return
                    handleSettingsNavigate(`org/${orgId}/`)
                  }}
                  onOpenLinkDevice={() => setDetailsPath('/link-device')}
                />

                <SessionActionsSection
                  sx={nativeChildSpacing.s3}
                  session={session}
                  sessionIdx={sessionIdx}
                  isLocal={isLocal}
                  showTransfer={showTransfer}
                  showLogout={!isLocal}
                  loggingOut={logout.loggingOut}
                  logoutOpen={logout.logoutOpen}
                  dangerZoneOpen={dangerZoneOpen}
                  onDangerZoneOpenChange={setDangerZoneOpen}
                  onLogoutOpenChange={logout.setLogoutOpen}
                  onLogoutClick={() => logout.setLogoutOpen(true)}
                  onLogoutConfirm={() => void logout.confirmLogout()}
                  onUpgradeToCloud={() => handleSettingsNavigate('plan')}
                  onDeleteAccountRedirect={handleCloseDetails}
                />
              </div>
            </div>
            {scrollCue.showCue && (
              <div sx={stylexStyles2.rule3}>
                <button
                  type="button"
                  aria-label="Scroll down for more settings"
                  onClick={scrollCue.scrollMore}
                  sx={stylexStyles2.rule4}
                >
                  More settings
                  <LuChevronDown sx={stylexStyles.rule1} />
                </button>
              </div>
            )}
          </div>
        </Route>
      </Routes>
    </Router>
  )
}

// DetailsStatus centers a loading or error message in the details panel.
function DetailsStatus({
  destructive,
  children,
}: {
  destructive?: boolean
  children: React.ReactNode
}) {
  return (
    <div sx={stylexStyles2.rule5}>
      <div
        sx={[
          stylexStyles2.rule6,
          destructive ? stylexStyles2.rule7 : stylexStyles2.rule8,
        ]}
      >
        {children}
      </div>
    </div>
  )
}

interface SettingsSectionsProps {
  isLocal: boolean
  accountResource: Resource<Account>
  session: Session | null | undefined
  currentDisplayName: string
  sectionProps: SectionProps
  onNavigateToPath: (path: string) => void
  onOpenOrganization: (orgId: string) => void
  onOpenLinkDevice: () => void
}

// SettingsSections renders the collapsible settings sections for the local or
// cloud account.
function SettingsSections({
  sx,
  isLocal,
  accountResource,
  session,
  currentDisplayName,
  sectionProps,
  onNavigateToPath,
  onOpenOrganization,
  onOpenLinkDevice,
}: SettingsSectionsProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const account = accountResource.value
  const retainStepUp = !isLocal && !!account

  return (
    <>
      <SessionSyncStatusSummary sx={sx} />
      <StorageHealthSection
        sx={sx}
        {...sectionProps('storage')}
        onNavigateToPath={onNavigateToPath}
      />

      {isLocal && (
        <DisplayNameCard
          sx={sx}
          session={session}
          currentDisplayName={currentDisplayName}
          {...sectionProps('account')}
        />
      )}

      {!isLocal && account ? (
        <AccountDashboardStateProvider account={accountResource}>
          <AuthMethodsSection
            sx={sx}
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('auth-methods')}
          />
          <EmailSection sx={sx} {...sectionProps('email')} />
          <SecuritySection
            sx={sx}
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('security')}
          />
          <SessionsSection
            sx={sx}
            account={accountResource}
            isLocal={isLocal}
            retainStepUp={retainStepUp}
            onLinkDeviceClick={onOpenLinkDevice}
            {...sectionProps('sessions')}
          />
        </AccountDashboardStateProvider>
      ) : (
        <>
          <SecuritySection
            sx={sx}
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('security')}
          />
          {account && (
            <SessionsSection
              sx={sx}
              account={accountResource}
              isLocal={isLocal}
              retainStepUp={retainStepUp}
              onLinkDeviceClick={onOpenLinkDevice}
              {...sectionProps('sessions')}
            />
          )}
        </>
      )}
      <OrganizationsSection
        sx={sx}
        {...sectionProps('orgs')}
        onNavigateToOrganization={onOpenOrganization}
      />
      <BillingSection
        sx={sx}
        isLocal={isLocal}
        {...sectionProps('billing')}
        onNavigateToPath={onNavigateToPath}
      />
      <CryptoKeysSection sx={sx} {...sectionProps('crypto')} />
      <IdentifiersSection sx={sx} {...sectionProps('identifiers')} />
    </>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
