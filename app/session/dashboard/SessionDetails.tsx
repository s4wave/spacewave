import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuCloud,
  LuLock,
  LuLogOut,
  LuMerge,
  LuPencil,
  LuSave,
  LuTerminal,
  LuTrash2,
  LuUserCog,
  LuX,
} from 'react-icons/lu'
import { RxPerson } from 'react-icons/rx'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { BillingSection } from '@s4wave/app/billing/BillingSection.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
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
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Input } from '@s4wave/web/ui/input.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import {
  WatchLocalDisplayNameRequest,
  WatchLocalDisplayNameResponse,
} from '@s4wave/sdk/session/local-session.pb.js'

import { LinkDeviceWizard } from '../setup/LinkDeviceWizard.js'
import { AccountDashboardStateProvider } from './AccountDashboardStateContext.js'
import { AuthMethodsSection } from './AuthMethodsSection.js'
import { CryptoKeysSection } from './CryptoKeysSection.js'
import { DeleteAccountDialog } from './DeleteAccountDialog.js'
import { DeleteSpaceEscapeHatchDialog } from './DeleteSpaceEscapeHatchDialog.js'
import { EmailSection } from './EmailSection.js'
import { IdentifiersSection } from './IdentifiersSection.js'
import { OrganizationsSection } from './OrganizationsSection.js'
import { SecuritySection } from './SecuritySection.js'
import { SessionsSection } from './SessionsSection.js'
import { SessionSyncStatusSummary } from './SessionSyncStatusSummary.js'

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
  | 'crypto'
  | 'identifiers'
  | null

// SessionDetails displays account/session information and actions.
export function SessionDetails({
  onCloseClick,
  onChangeAccountClick,
}: SessionDetailsProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const [locking, setLocking] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [deleteAcctOpen, setDeleteAcctOpen] = useState(false)
  const [deleteSpaceOpen, setDeleteSpaceOpen] = useState(false)
  const [displayName, setDisplayName] = useState('')
  const [displayNameError, setDisplayNameError] = useState<string | null>(null)
  const [editingDisplayName, setEditingDisplayName] = useState(false)
  const [savingDisplayName, setSavingDisplayName] = useState(false)

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
  const retainStepUp = !isLocal && !!account
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
  const [openSection, setOpenSection] = useStateAtom<SessionOpenSection>(
    ns,
    'open-section',
    isLocal ? 'account' : 'auth-methods',
  )
  const localDisplayNameReq = useMemo<WatchLocalDisplayNameRequest>(
    () => ({}),
    [],
  )
  const localDisplayName = useWatchStateRpc(
    useCallback(
      (req: WatchLocalDisplayNameRequest, signal: AbortSignal) =>
        isLocal && session ?
          session.localProvider.watchDisplayName(req, signal)
        : null,
      [isLocal, session],
    ),
    localDisplayNameReq,
    WatchLocalDisplayNameRequest.equals,
    WatchLocalDisplayNameResponse.equals,
  )
  const currentDisplayName =
    localDisplayName?.displayName ?? metadata?.displayName ?? ''
  const displayNameChanged = displayName.trim() !== currentDisplayName

  // Hide logout for local sessions (no cloud session to revoke).
  const showLogout = !isLocal

  // Only show transfer button when there are multiple sessions to transfer between.
  const sessionsResource = useSessionList()
  const sessionCount = sessionsResource.value?.sessions?.length ?? 0
  const showTransfer = sessionCount > 1

  useEffect(() => {
    setDisplayName(currentDisplayName)
    setDisplayNameError(null)
    setEditingDisplayName(false)
  }, [currentDisplayName])

  useEffect(() => {
    return () => {
      setDetailsPath('/')
    }
  }, [setDetailsPath])

  const handleLogout = useCallback(async () => {
    if (!account || !peerId || peerId === 'Unknown') return
    setLoggingOut(true)
    await account.selfRevokeSession(peerId).catch(() => {})
    if (root && sessionIdx != null) {
      await root.deleteSession(sessionIdx).catch(() => {})
    }
    navigate({ path: '/sessions' })
  }, [account, navigate, peerId, root, sessionIdx])

  const handleLockClick = useCallback(async () => {
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
  }, [isPinMode, navigate, session, sessionIdx])

  const handleCloseDetails = useCallback(() => {
    setDetailsPath('/')
    onCloseClick?.()
  }, [onCloseClick, setDetailsPath])

  const handleDeleteAccount = useCallback(async () => {
    if (!isLocal && sessionIdx != null) {
      setDeleteAcctOpen(false)
      handleCloseDetails()
      navigateSession({ path: 'delete-account' })
      return
    }
    if (session && sessionIdx != null) {
      await session.deleteAccount(sessionIdx)
    }
    navigate({ path: '/sessions' })
  }, [
    handleCloseDetails,
    isLocal,
    navigate,
    navigateSession,
    session,
    sessionIdx,
  ])

  const handleSaveDisplayName = useCallback(async () => {
    if (!isLocal || !session || savingDisplayName || !displayNameChanged) return
    setDisplayNameError(null)
    setSavingDisplayName(true)
    try {
      await session.localProvider.setDisplayName({
        displayName: displayName.trim(),
      })
      setEditingDisplayName(false)
    } catch (err) {
      setDisplayNameError(
        err instanceof Error ? err.message : 'Failed to update account name',
      )
    } finally {
      setSavingDisplayName(false)
    }
  }, [displayName, displayNameChanged, isLocal, savingDisplayName, session])

  const handleStartDisplayNameEdit = useCallback(() => {
    if (!isLocal || savingDisplayName) return
    setDisplayName(currentDisplayName)
    setDisplayNameError(null)
    setEditingDisplayName(true)
  }, [currentDisplayName, isLocal, savingDisplayName])

  const handleCancelDisplayNameEdit = useCallback(() => {
    setDisplayName(currentDisplayName)
    setDisplayNameError(null)
    setEditingDisplayName(false)
  }, [currentDisplayName])

  const handleSectionOpenChange = useCallback(
    (section: Exclude<SessionOpenSection, null>) => (open: boolean) => {
      setOpenSection(open ? section : null)
    },
    [setOpenSection],
  )

  const handleDetailsNavigate = useCallback(
    (to: To) => {
      setDetailsPath((curr) => resolvePath(curr, to))
    },
    [setDetailsPath],
  )

  const handleOpenLinkDevice = useCallback(() => {
    setDetailsPath('/link-device')
  }, [setDetailsPath])

  const handleBackToDetails = useCallback(() => {
    setDetailsPath('/')
  }, [setDetailsPath])

  const handleChangeAccount = useCallback(() => {
    setDetailsPath('/')
    onChangeAccountClick?.()
  }, [onChangeAccountClick, setDetailsPath])

  const handleOpenOrganization = useCallback(
    (orgId: string) => {
      if (!orgId) return
      handleCloseDetails()
      navigateSession({ path: `org/${orgId}/` })
    },
    [handleCloseDetails, navigateSession],
  )

  const handleBillingNavigate = useCallback(
    (path: string) => {
      handleCloseDetails()
      navigateSession({ path })
    },
    [handleCloseDetails, navigateSession],
  )

  if (loading) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center">
        <div className="text-foreground-alt text-xs select-none">
          Loading session info...
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center">
        <div className="text-destructive text-xs select-none">
          Error: {error.message}
        </div>
      </div>
    )
  }

  return (
    <Router path={detailsPath} onNavigate={handleDetailsNavigate}>
      <Routes fullPath>
        <Route path="/link-device">
          <LinkDeviceWizard
            exitPath="/"
            topLeft={
              <button
                onClick={handleBackToDetails}
                className="text-foreground-alt hover:text-brand flex items-center gap-2 text-sm transition-colors"
              >
                <LuArrowLeft className="h-4 w-4" />
                <span className="select-none">Back</span>
              </button>
            }
          />
        </Route>
        <Route path="/">
          <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
            <div className="border-foreground/8 flex min-h-9 shrink-0 items-center justify-between gap-3 border-b px-4 py-2">
              <div className="text-foreground flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold select-none">
                <RxPerson className="h-4 w-4" />
                <span className="min-w-0 truncate tracking-tight">
                  {metadata?.displayName || peerId?.slice(-12) || 'Session'}
                </span>
              </div>
              <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DashboardButton
                      icon={<LuUserCog className="h-4 w-4" />}
                      onClick={handleChangeAccount}
                    >
                      <span className="hidden md:inline">Change Account</span>
                    </DashboardButton>
                  </TooltipTrigger>
                  <TooltipContent side="bottom" className="md:hidden">
                    Change Account
                  </TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DashboardButton
                      icon={<LuLock className="h-4 w-4" />}
                      onClick={() => void handleLockClick()}
                      disabled={!lockState || locking}
                    >
                      <span className="hidden md:inline">Lock</span>
                    </DashboardButton>
                  </TooltipTrigger>
                  <TooltipContent side="bottom" className="md:hidden">
                    Lock
                  </TooltipContent>
                </Tooltip>
                {showLogout && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <DashboardButton
                        icon={<LuLogOut className="h-4 w-4" />}
                        className="text-destructive hover:bg-destructive/10"
                        onClick={() => void handleLogout()}
                        disabled={loggingOut}
                      >
                        <span className="hidden md:inline">
                          {loggingOut ? 'Logging out...' : 'Logout'}
                        </span>
                      </DashboardButton>
                    </TooltipTrigger>
                    <TooltipContent side="bottom">
                      Logout (revoke cloud session)
                    </TooltipContent>
                  </Tooltip>
                )}
                {onCloseClick && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <DashboardButton
                        icon={<LuX className="h-4 w-4" />}
                        onClick={handleCloseDetails}
                      />
                    </TooltipTrigger>
                    <TooltipContent side="bottom">Close</TooltipContent>
                  </Tooltip>
                )}
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
              <div className="space-y-3">
                <SessionSyncStatusSummary />

                {isLocal && (
                  <CollapsibleSection
                    title="Account"
                    icon={<LuUserCog className="h-3.5 w-3.5" />}
                    open={openSection === 'account'}
                    onOpenChange={handleSectionOpenChange('account')}
                  >
                    <InfoCard>
                      <div className="space-y-2">
                        <div>
                          <label className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
                            Display Name
                          </label>
                          {editingDisplayName ?
                            <>
                              <div className="flex items-center gap-2">
                                <Input
                                  value={displayName}
                                  onChange={(e) => {
                                    setDisplayName(e.target.value)
                                    setDisplayNameError(null)
                                  }}
                                  onKeyDown={(e) => {
                                    if (e.key === 'Enter') {
                                      e.preventDefault()
                                      void handleSaveDisplayName()
                                    }
                                    if (e.key === 'Escape') {
                                      e.preventDefault()
                                      handleCancelDisplayNameEdit()
                                    }
                                  }}
                                  placeholder="Name this local account"
                                  aria-label="Display Name"
                                  className="text-sm"
                                />
                                <DashboardButton
                                  icon={<LuSave className="h-3 w-3" />}
                                  onClick={() => void handleSaveDisplayName()}
                                  disabled={
                                    !displayNameChanged || savingDisplayName
                                  }
                                >
                                  {savingDisplayName ? 'Saving...' : 'Save'}
                                </DashboardButton>
                                <DashboardButton
                                  icon={<LuX className="h-3 w-3" />}
                                  onClick={handleCancelDisplayNameEdit}
                                  disabled={savingDisplayName}
                                >
                                  Cancel
                                </DashboardButton>
                              </div>
                              {displayNameError && (
                                <div className="text-destructive mt-1 text-xs">
                                  {displayNameError}
                                </div>
                              )}
                            </>
                          : <div className="flex items-center justify-between gap-2">
                              <div
                                className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-xs transition-colors"
                                role="button"
                                tabIndex={0}
                                onDoubleClick={handleStartDisplayNameEdit}
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault()
                                    handleStartDisplayNameEdit()
                                  }
                                }}
                              >
                                {currentDisplayName || 'Unnamed account'}
                              </div>
                              <DashboardButton
                                icon={<LuPencil className="h-3 w-3" />}
                                onClick={handleStartDisplayNameEdit}
                              >
                                Edit
                              </DashboardButton>
                            </div>
                          }
                        </div>
                      </div>
                    </InfoCard>
                  </CollapsibleSection>
                )}

                {!isLocal && account ?
                  <AccountDashboardStateProvider account={accountResource}>
                    <AuthMethodsSection
                      account={accountResource}
                      retainStepUp={retainStepUp}
                      open={openSection === 'auth-methods'}
                      onOpenChange={handleSectionOpenChange('auth-methods')}
                    />
                    <EmailSection
                      open={openSection === 'email'}
                      onOpenChange={handleSectionOpenChange('email')}
                    />
                    <SecuritySection
                      account={accountResource}
                      retainStepUp={retainStepUp}
                      open={openSection === 'security'}
                      onOpenChange={handleSectionOpenChange('security')}
                    />
                    <SessionsSection
                      account={accountResource}
                      isLocal={isLocal}
                      retainStepUp={retainStepUp}
                      open={openSection === 'sessions'}
                      onOpenChange={handleSectionOpenChange('sessions')}
                      onLinkDeviceClick={handleOpenLinkDevice}
                    />
                  </AccountDashboardStateProvider>
                : <>
                    <SecuritySection
                      account={accountResource}
                      retainStepUp={retainStepUp}
                      open={openSection === 'security'}
                      onOpenChange={handleSectionOpenChange('security')}
                    />
                    {account && (
                      <SessionsSection
                        account={accountResource}
                        isLocal={isLocal}
                        retainStepUp={retainStepUp}
                        open={openSection === 'sessions'}
                        onOpenChange={handleSectionOpenChange('sessions')}
                        onLinkDeviceClick={handleOpenLinkDevice}
                      />
                    )}
                  </>
                }
                <OrganizationsSection
                  open={openSection === 'orgs'}
                  onOpenChange={handleSectionOpenChange('orgs')}
                  onNavigateToOrganization={handleOpenOrganization}
                />
                <BillingSection
                  isLocal={isLocal}
                  open={openSection === 'billing'}
                  onOpenChange={handleSectionOpenChange('billing')}
                  onNavigateToPath={handleBillingNavigate}
                />
                <CryptoKeysSection
                  open={openSection === 'crypto'}
                  onOpenChange={handleSectionOpenChange('crypto')}
                />

                <IdentifiersSection
                  open={openSection === 'identifiers'}
                  onOpenChange={handleSectionOpenChange('identifiers')}
                />

                <section>
                  <div className="mb-2 flex items-center justify-between">
                    <h2 className="text-foreground-alt text-xs font-medium select-none">
                      Actions
                    </h2>
                  </div>

                  <div className="space-y-2">
                    {isLocal && sessionIdx != null && (
                      <button
                        onClick={() => navigateSession({ path: 'plan' })}
                        className={cn(
                          'border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                        )}
                      >
                        <div className="bg-foreground/10 group-hover:bg-brand/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                          <LuCloud className="text-foreground-alt group-hover:text-brand h-3.5 w-3.5 transition-colors" />
                        </div>
                        <div className="flex min-w-0 flex-1 flex-col">
                          <h4 className="text-foreground text-xs font-medium select-none">
                            Upgrade to Cloud
                          </h4>
                          <p className="text-foreground-alt text-xs select-none">
                            Sync across devices with Spacewave Cloud
                          </p>
                        </div>
                      </button>
                    )}

                    <button
                      onClick={() => navigateSession({ path: 'settings/cli' })}
                      className={cn(
                        'border-foreground/20 bg-foreground/5 hover:border-foreground/35 hover:bg-foreground/10 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                      )}
                    >
                      <div className="bg-foreground/10 group-hover:bg-foreground/15 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                        <LuTerminal className="text-foreground-alt h-3.5 w-3.5 transition-colors" />
                      </div>
                      <div className="flex min-w-0 flex-1 flex-col">
                        <h4 className="text-foreground text-xs font-medium select-none">
                          Command Line
                        </h4>
                        <p className="text-foreground-alt text-xs select-none">
                          Connect the spacewave CLI to this session
                        </p>
                      </div>
                    </button>

                    {showTransfer && (
                      <button
                        onClick={() =>
                          navigateSession({ path: 'settings/transfer' })
                        }
                        className={cn(
                          'border-foreground/20 bg-foreground/5 hover:border-foreground/35 hover:bg-foreground/10 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                        )}
                      >
                        <div className="bg-foreground/10 group-hover:bg-foreground/15 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                          <LuMerge className="text-foreground-alt h-3.5 w-3.5 transition-colors" />
                        </div>
                        <div className="flex min-w-0 flex-1 flex-col">
                          <h4 className="text-foreground text-xs font-medium select-none">
                            Transfer Sessions
                          </h4>
                          <p className="text-foreground-alt text-xs select-none">
                            Merge spaces from another session into this one
                          </p>
                        </div>
                      </button>
                    )}

                    {showLogout && (
                      <button
                        onClick={() => void handleLogout()}
                        disabled={loggingOut}
                        className={cn(
                          'border-warning/30 bg-warning/5 hover:border-warning hover:bg-warning/10 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                          loggingOut && 'cursor-not-allowed opacity-50',
                        )}
                      >
                        <div className="bg-warning/20 group-hover:bg-warning/30 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                          <LuLogOut className="text-warning h-3.5 w-3.5" />
                        </div>
                        <div className="flex min-w-0 flex-1 flex-col">
                          <h4 className="text-warning text-xs font-medium select-none">
                            {loggingOut ? 'Logging out...' : 'Log Out'}
                          </h4>
                          <p className="text-warning/80 text-xs select-none">
                            Sign out and remove session data
                          </p>
                        </div>
                      </button>
                    )}

                    <CollapsibleSection
                      title="Danger Zone"
                      open={dangerZoneOpen}
                      onOpenChange={setDangerZoneOpen}
                    >
                      <div className="space-y-2">
                        <button
                          onClick={() => setDeleteSpaceOpen(true)}
                          disabled={!session}
                          className={cn(
                            'border-destructive/20 bg-destructive/5 hover:border-destructive/40 hover:bg-destructive/10 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                            !session && 'cursor-not-allowed opacity-50',
                          )}
                        >
                          <div className="bg-destructive/10 group-hover:bg-destructive/15 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                            <LuTrash2 className="text-destructive h-3.5 w-3.5 transition-colors" />
                          </div>
                          <div className="flex min-w-0 flex-1 flex-col">
                            <h4 className="text-destructive text-xs font-medium transition-colors select-none">
                              Delete a Space
                            </h4>
                            <p className="text-destructive/80 text-xs transition-colors select-none">
                              Permanently remove a broken space without opening
                              it
                            </p>
                          </div>
                        </button>

                        <button
                          onClick={() => setDeleteAcctOpen(true)}
                          className={cn(
                            'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive hover:text-destructive-foreground group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                          )}
                        >
                          <div className="bg-destructive/20 group-hover:bg-destructive-foreground/20 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
                            <LuTrash2 className="text-destructive group-hover:text-destructive-foreground h-3.5 w-3.5 transition-colors" />
                          </div>
                          <div className="flex min-w-0 flex-1 flex-col">
                            <h4 className="text-destructive group-hover:text-destructive-foreground text-xs font-medium transition-colors select-none">
                              {isLocal ? 'Delete Local Data' : 'Delete Account'}
                            </h4>
                            <p className="text-destructive/80 group-hover:text-destructive-foreground/80 text-xs transition-colors select-none">
                              {isLocal ?
                                'Permanently remove this account and all local data'
                              : 'Permanently delete this account and all data'}
                            </p>
                          </div>
                        </button>
                      </div>
                    </CollapsibleSection>
                  </div>

                  <DeleteAccountDialog
                    open={deleteAcctOpen}
                    onOpenChange={setDeleteAcctOpen}
                    isCloud={!isLocal}
                    onConfirm={handleDeleteAccount}
                  />
                  <DeleteSpaceEscapeHatchDialog
                    open={deleteSpaceOpen}
                    onOpenChange={setDeleteSpaceOpen}
                    session={session ?? null}
                  />
                </section>
              </div>
            </div>
          </div>
        </Route>
      </Routes>
    </Router>
  )
}
