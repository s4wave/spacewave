import { lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuChevronDown } from 'react-icons/lu'
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
import { cn } from '@s4wave/web/style/utils.js'
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
          <div className="bg-background-primary relative flex h-full w-full flex-col overflow-hidden">
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
              className="min-h-0 flex-1 overflow-auto px-4 pt-3 pb-3"
              onScroll={scrollCue.update}
            >
              <div className="space-y-3">
                <SettingsSections
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
              <div className="border-foreground/8 flex shrink-0 justify-end border-t px-3 py-1">
                <button
                  type="button"
                  aria-label="Scroll down for more settings"
                  onClick={scrollCue.scrollMore}
                  className="border-foreground/10 bg-background-card/90 text-foreground-alt hover:text-foreground flex min-h-11 items-center gap-1 rounded-full border px-3 text-xs shadow-md transition-colors"
                >
                  More settings
                  <LuChevronDown className="size-3" />
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
    <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center">
      <div
        className={cn(
          'text-xs select-none',
          destructive ? 'text-destructive' : 'text-foreground-alt',
        )}
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
  isLocal,
  accountResource,
  session,
  currentDisplayName,
  sectionProps,
  onNavigateToPath,
  onOpenOrganization,
  onOpenLinkDevice,
}: SettingsSectionsProps) {
  const account = accountResource.value
  const retainStepUp = !isLocal && !!account

  return (
    <>
      <SessionSyncStatusSummary />
      <StorageHealthSection
        {...sectionProps('storage')}
        onNavigateToPath={onNavigateToPath}
      />

      {isLocal && (
        <DisplayNameCard
          session={session}
          currentDisplayName={currentDisplayName}
          {...sectionProps('account')}
        />
      )}

      {!isLocal && account ? (
        <AccountDashboardStateProvider account={accountResource}>
          <AuthMethodsSection
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('auth-methods')}
          />
          <EmailSection {...sectionProps('email')} />
          <SecuritySection
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('security')}
          />
          <SessionsSection
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
            account={accountResource}
            retainStepUp={retainStepUp}
            {...sectionProps('security')}
          />
          {account && (
            <SessionsSection
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
        {...sectionProps('orgs')}
        onNavigateToOrganization={onOpenOrganization}
      />
      <BillingSection
        isLocal={isLocal}
        {...sectionProps('billing')}
        onNavigateToPath={onNavigateToPath}
      />
      <CryptoKeysSection {...sectionProps('crypto')} />
      <IdentifiersSection {...sectionProps('identifiers')} />
    </>
  )
}
