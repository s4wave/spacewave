import { useCallback, useState, type ReactNode } from 'react'
import {
  LuBuilding2,
  LuCircleAlert,
  LuLogOut,
  LuUsers,
  LuLink,
  LuPencil,
  LuPlus,
  LuRefreshCw,
  LuSave,
  LuSettings,
  LuShieldAlert,
  LuTriangleAlert,
  LuFingerprint,
  LuX,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import type {
  OrganizationRootStateInfo,
  SharedObjectMutationPermission,
  WatchOrganizationStateResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { ORG_ROLE_OWNER } from './org-constants.js'
import { OrgMemberList } from './OrgMemberList.js'
import { OrgInviteSection } from './OrgInviteSection.js'
import { OrgActionsSection } from './OrgActionsSection.js'
import { OrgBillingSection } from './OrgBillingSection.js'

type OrgOpenSection =
  | 'recovery'
  | 'members'
  | 'invites'
  | 'settings'
  | 'billing'
  | 'identifiers'
  | null

export interface OrganizationDetailsProps {
  orgId: string
  orgState: WatchOrganizationStateResponse | null
  orgName?: string
  degraded?: boolean
  isOwner: boolean
  onCloseClick?: () => void
}

function getRoleLabel(role?: string): string {
  if (role === ORG_ROLE_OWNER) return 'Owner'
  return 'Member'
}

function getRecoverySummary(health?: SharedObjectHealth | null): {
  tone: 'loading' | 'degraded' | 'closed'
  title: string
  description: string
  hint: string
} {
  if (!health) {
    return {
      tone: 'closed',
      title: 'Organization root shared object unavailable',
      description:
        'The organization dashboard is running in degraded mode because the org root shared object could not load.',
      hint: 'Repair retries the normal recovery path. Reinitialize is destructive and rewrites the same shared object id in place.',
    }
  }

  if (health.status === SharedObjectHealthStatus.LOADING) {
    return {
      tone: 'loading',
      title: 'Checking organization root shared object',
      description:
        'Alpha is still verifying and mounting the organization root shared object.',
      hint: '',
    }
  }

  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED
  ) {
    return {
      tone: 'closed',
      title: 'Organization root initial state rejected',
      description:
        'The organization root failed verification, so Alpha kept the dashboard available in degraded mode instead of looping.',
      hint: 'Repair retries owner-side recovery on the current shared object id. Reinitialize discards the broken state and reseeds the same id in place.',
    }
  }

  if (health.commonReason === SharedObjectHealthCommonReason.BLOCK_NOT_FOUND) {
    return {
      tone: 'closed',
      title: 'Organization root data missing',
      description:
        'A required block for the organization root shared object could not be found.',
      hint: 'Retry if replication may still be catching up. Otherwise repair or reinitialize the organization root.',
    }
  }

  if (health.status === SharedObjectHealthStatus.DEGRADED) {
    return {
      tone: 'degraded',
      title: 'Organization root degraded',
      description:
        'The organization root is partially available, but Alpha detected a recoverable problem.',
      hint: 'Use repair first when you want Alpha to retry the normal recovery path without discarding the current state.',
    }
  }

  return {
    tone: 'closed',
    title: 'Organization root shared object unavailable',
    description:
      'Alpha could not mount the organization root shared object, so the dashboard stays available in degraded mode.',
    hint: 'Repair retries the normal owner recovery path. Reinitialize is destructive and rewrites the same shared object id in place.',
  }
}

function getRecoveryPermission(
  permission: SharedObjectMutationPermission | null | undefined,
  isOwner: boolean,
): SharedObjectMutationPermission {
  if (permission) {
    return permission
  }
  return {
    canRepair: isOwner,
    canReinitialize: isOwner,
    disabledReason:
      isOwner ? '' : (
        'Only organization owners can repair or reinitialize this shared object.'
      ),
  }
}

function RecoveryActionButton({
  label,
  icon,
  onClick,
  disabled,
  disabledReason,
  destructive = false,
}: {
  label: string
  icon: ReactNode
  onClick: () => void
  disabled: boolean
  disabledReason: string
  destructive?: boolean
}) {
  const button = (
    <DashboardButton
      icon={icon}
      onClick={onClick}
      disabled={disabled || !!disabledReason}
      className={
        destructive ?
          'text-destructive hover:bg-destructive/10 hover:text-destructive'
        : undefined
      }
    >
      {label}
    </DashboardButton>
  )
  if (!disabledReason) {
    return button
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{button}</span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs">
        {disabledReason}
      </TooltipContent>
    </Tooltip>
  )
}

// OrganizationDetails renders the overlay panel for organization management.
// Mirrors SessionDetails: header bar + scrollable collapsible sections.
export function OrganizationDetails({
  orgId,
  orgState,
  orgName: fallbackOrgName,
  degraded = false,
  isOwner,
  onCloseClick,
}: OrganizationDetailsProps) {
  const session = SessionContext.useContext().value
  const navigateSession = useSessionNavigate()
  const ns = useStateNamespace(['org-details'])

  const info = orgState?.organization
  const rootState = orgState?.rootState
  const members = orgState?.members ?? []
  const invites = orgState?.invites ?? []
  const spaces = orgState?.spaces ?? []
  const orgName = info?.displayName || fallbackOrgName || 'Organization'
  const roleLabel = getRoleLabel(
    info?.role ?? (isOwner ? ORG_ROLE_OWNER : 'org:member'),
  )
  const recoverySummary = getRecoverySummary(rootState?.health)
  const recoveryPermission = getRecoveryPermission(
    rootState?.mutationPermission,
    isOwner,
  )
  const rootSharedObjectId = rootState?.sharedObjectId || orgId

  const [openSection, setOpenSection] = useStateAtom<OrgOpenSection>(
    ns,
    'open-section',
    degraded ? 'recovery' : 'members',
  )
  const [mutationPending, setMutationPending] = useState(false)
  const [mutationError, setMutationError] = useState('')
  const [confirmingReinitialize, setConfirmingReinitialize] = useState(false)

  const handleSectionOpenChange = useCallback(
    (section: Exclude<OrgOpenSection, null>) => (open: boolean) => {
      setOpenSection(open ? section : null)
    },
    [setOpenSection],
  )

  // Member management
  const [creatingInvite, setCreatingInvite] = useState(false)

  const handleRemoveMember = useCallback(
    async (memberId: string) => {
      if (!session) return
      try {
        await session.spacewave.removeOrgMember(orgId, memberId)
      } catch (err) {
        const msg = err instanceof Error ? err.message : ''
        if (msg.toLowerCase().includes('not found')) {
          toast.info('Member already removed. Refreshing...', {
            duration: 3000,
          })
          return
        }
        throw err
      }
    },
    [session, orgId],
  )

  const handleCreateInvite = useCallback(async () => {
    if (!session || creatingInvite) return
    setCreatingInvite(true)
    try {
      await session.spacewave.createOrgInvite({ orgId, type: 'code' })
    } finally {
      setCreatingInvite(false)
    }
  }, [session, creatingInvite, orgId])

  const handleRevokeInvite = useCallback(
    async (inviteId: string) => {
      if (!session) return
      try {
        await session.spacewave.revokeOrgInvite(orgId, inviteId)
      } catch (err) {
        const msg = err instanceof Error ? err.message : ''
        if (msg.toLowerCase().includes('not found')) {
          toast.info('Invite already revoked. Refreshing...', {
            duration: 3000,
          })
          return
        }
        throw err
      }
    },
    [session, orgId],
  )

  const handleLeave = useCallback(async () => {
    if (!session) return
    await session.spacewave.leaveOrganization(orgId)
    navigateSession({ path: '' })
  }, [session, orgId, navigateSession])

  // Rename state
  const [renameValue, setRenameValue] = useState('')
  const [renaming, setRenaming] = useState(false)
  const [renameSaving, setRenameSaving] = useState(false)

  const handleRenameStart = useCallback(() => {
    setRenameValue(orgName)
    setRenaming(true)
  }, [orgName])

  const handleRenameSave = useCallback(async () => {
    if (!session || renameSaving || renameValue.trim() === orgName) return
    setRenameSaving(true)
    try {
      await session.spacewave.updateOrganization(orgId, renameValue.trim())
      setRenaming(false)
    } finally {
      setRenameSaving(false)
    }
  }, [session, orgId, renameValue, orgName, renameSaving])

  const handleRenameCancel = useCallback(() => {
    setRenaming(false)
    setRenameValue('')
  }, [])

  const runRecoveryAction = useCallback(
    async (kind: 'repair' | 'reinitialize') => {
      if (!session || !rootSharedObjectId || mutationPending) return
      setMutationPending(true)
      setMutationError('')
      try {
        if (kind === 'repair') {
          await session.spacewave.repairSharedObject(rootSharedObjectId)
        } else {
          await session.spacewave.reinitializeSharedObject(rootSharedObjectId)
        }
      } catch (err) {
        setMutationError(err instanceof Error ? err.message : 'Action failed')
      } finally {
        setMutationPending(false)
      }
    },
    [mutationPending, rootSharedObjectId, session],
  )

  if (!orgState && !degraded) {
    return (
      <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading organization',
              detail: 'Reading the organization state and sharing details.',
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <div className="border-foreground/8 flex min-h-9 shrink-0 items-center justify-between gap-3 border-b px-4 py-2">
        <div className="text-foreground flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold select-none">
          <div className="bg-brand/10 text-brand flex h-5 w-5 shrink-0 items-center justify-center rounded">
            <LuBuilding2 className="h-3 w-3" />
          </div>
          <span className="min-w-0 truncate tracking-tight">{orgName}</span>
          <span className="text-foreground-alt/50 text-xs font-normal">
            {roleLabel}
          </span>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
          {!isOwner && info && (
            <Tooltip>
              <TooltipTrigger asChild>
                <DashboardButton
                  icon={<LuLogOut className="h-4 w-4" />}
                  className="text-destructive hover:bg-destructive/10"
                  onClick={() => void handleLeave()}
                >
                  <span className="hidden md:inline">Leave</span>
                </DashboardButton>
              </TooltipTrigger>
              <TooltipContent side="bottom">Leave organization</TooltipContent>
            </Tooltip>
          )}
          {onCloseClick && (
            <Tooltip>
              <TooltipTrigger asChild>
                <DashboardButton
                  icon={<LuX className="h-4 w-4" />}
                  onClick={onCloseClick}
                />
              </TooltipTrigger>
              <TooltipContent side="bottom">Close</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div className="space-y-3">
          {degraded && (
            <CollapsibleSection
              title="Recovery"
              icon={<LuCircleAlert className="h-3.5 w-3.5" />}
              open={openSection === 'recovery'}
              onOpenChange={handleSectionOpenChange('recovery')}
            >
              <InfoCard>
                <div className="space-y-3">
                  <div
                    className={cn(
                      'rounded-md border px-3 py-2',
                      recoverySummary.tone === 'loading' &&
                        'border-foreground/8 bg-background-card/30',
                      recoverySummary.tone === 'degraded' &&
                        'border-warning/20 bg-warning/5',
                      recoverySummary.tone === 'closed' &&
                        'border-destructive/20 bg-destructive/5',
                    )}
                  >
                    <div className="flex items-start gap-2">
                      <div
                        className={cn(
                          'flex h-7 w-7 shrink-0 items-center justify-center rounded-md',
                          recoverySummary.tone === 'loading' &&
                            'bg-foreground/5',
                          recoverySummary.tone === 'degraded' &&
                            'bg-warning/10',
                          recoverySummary.tone === 'closed' &&
                            'bg-destructive/10',
                        )}
                      >
                        {recoverySummary.tone === 'loading' ?
                          <Spinner className="text-foreground" />
                        : recoverySummary.tone === 'degraded' ?
                          <LuTriangleAlert className="text-warning h-4 w-4" />
                        : <LuShieldAlert className="text-destructive h-4 w-4" />
                        }
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-foreground text-xs font-medium">
                          {recoverySummary.title}
                        </p>
                        <p className="text-foreground-alt/65 mt-1 text-xs">
                          {recoverySummary.description}
                        </p>
                        {recoverySummary.hint && (
                          <p className="text-foreground-alt/60 mt-2 text-[11px]">
                            {recoverySummary.hint}
                          </p>
                        )}
                        {rootState?.health?.error && (
                          <div className="border-foreground/8 bg-background-card/30 text-foreground-alt/70 mt-2 rounded-md border px-2 py-1.5 text-[0.65rem] break-words whitespace-pre-wrap">
                            {rootState.health.error}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>

                  <div className="border-foreground/8 bg-background-card/20 rounded-md border px-3 py-2">
                    <div className="text-foreground-alt/45 text-[0.58rem] font-medium tracking-widest uppercase">
                      Remediation
                    </div>
                    <p className="text-foreground-alt/65 mt-1 text-[0.72rem]">
                      This works in place on the canonical organization root
                      shared object at the current org-owned id.
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <RecoveryActionButton
                        label={mutationPending ? 'Repairing...' : 'Repair'}
                        icon={<LuRefreshCw className="h-3.5 w-3.5" />}
                        onClick={() => {
                          setConfirmingReinitialize(false)
                          void runRecoveryAction('repair')
                        }}
                        disabled={mutationPending}
                        disabledReason={
                          recoveryPermission.canRepair ? '' : (
                            (recoveryPermission.disabledReason ?? '')
                          )
                        }
                      />
                      <RecoveryActionButton
                        label="Reinitialize"
                        icon={<LuShieldAlert className="h-3.5 w-3.5" />}
                        onClick={() => setConfirmingReinitialize(true)}
                        disabled={mutationPending}
                        disabledReason={
                          recoveryPermission.canReinitialize ? '' : (
                            (recoveryPermission.disabledReason ?? '')
                          )
                        }
                        destructive={true}
                      />
                    </div>
                    {confirmingReinitialize && (
                      <div className="border-destructive/20 bg-destructive/5 mt-3 rounded-md border px-3 py-2">
                        <div className="text-destructive/80 text-[0.58rem] font-medium tracking-widest uppercase">
                          Confirm Reinitialize
                        </div>
                        <p className="text-foreground-alt/70 mt-1 text-[0.72rem]">
                          Reinitialize is destructive. It rewrites the
                          organization root shared object in place on the same
                          shared object id and canonical org route.
                        </p>
                        <div className="mt-3 flex flex-wrap gap-2">
                          <DashboardButton
                            icon={<LuX className="h-3.5 w-3.5" />}
                            onClick={() => setConfirmingReinitialize(false)}
                          >
                            Cancel
                          </DashboardButton>
                          <DashboardButton
                            icon={<LuShieldAlert className="h-3.5 w-3.5" />}
                            onClick={() => {
                              setConfirmingReinitialize(false)
                              void runRecoveryAction('reinitialize')
                            }}
                            disabled={mutationPending}
                            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                          >
                            {mutationPending ?
                              'Reinitializing...'
                            : 'Confirm reinitialize'}
                          </DashboardButton>
                        </div>
                      </div>
                    )}
                    {mutationError && (
                      <p className="text-destructive mt-2 text-[0.68rem]">
                        {mutationError}
                      </p>
                    )}
                    <p className="text-foreground-alt/55 mt-2 text-[0.68rem]">
                      Shared object ID: {rootSharedObjectId}
                    </p>
                  </div>
                </div>
              </InfoCard>
            </CollapsibleSection>
          )}
          {orgState && (
            <CollapsibleSection
              title="Members"
              icon={<LuUsers className="h-3.5 w-3.5" />}
              open={openSection === 'members'}
              onOpenChange={handleSectionOpenChange('members')}
              badge={
                <span className="text-foreground-alt/40 text-[0.6rem]">
                  {members.length}
                </span>
              }
            >
              <p className="text-foreground-alt/60 mb-2 text-xs">
                Members are shown by username first. Their account ID stays
                underneath for review or copy.
              </p>
              <OrgMemberList
                members={members}
                isOwner={isOwner}
                onRemove={handleRemoveMember}
              />
            </CollapsibleSection>
          )}

          {isOwner && orgState && (
            <CollapsibleSection
              title="Invites"
              icon={<LuLink className="h-3.5 w-3.5" />}
              open={openSection === 'invites'}
              onOpenChange={handleSectionOpenChange('invites')}
              badge={
                invites.length > 0 ?
                  <span className="text-foreground-alt/40 text-[0.6rem]">
                    {invites.length}
                  </span>
                : undefined
              }
              headerActions={
                <button
                  type="button"
                  onClick={() => void handleCreateInvite()}
                  disabled={creatingInvite}
                  aria-label="Create invite"
                  title="Create invite"
                  className="text-foreground-alt hover:text-foreground flex h-4 w-4 items-center justify-center transition-colors disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <LuPlus className="h-3.5 w-3.5" />
                </button>
              }
            >
              <OrgInviteSection
                invites={invites}
                onRevoke={handleRevokeInvite}
              />
            </CollapsibleSection>
          )}

          {isOwner && orgState && (
            <CollapsibleSection
              title="Settings"
              icon={<LuSettings className="h-3.5 w-3.5" />}
              open={openSection === 'settings'}
              onOpenChange={handleSectionOpenChange('settings')}
            >
              <InfoCard>
                <div className="space-y-2">
                  <div>
                    <label className="text-foreground-alt mb-1 block text-[0.6rem] select-none">
                      Display Name
                    </label>
                    {renaming ?
                      <div className="flex items-center gap-2">
                        <input
                          type="text"
                          value={renameValue}
                          onChange={(e) => setRenameValue(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') void handleRenameSave()
                            if (e.key === 'Escape') handleRenameCancel()
                          }}
                          autoFocus
                          className={cn(
                            'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-2 py-1 text-xs transition-colors outline-none',
                            'focus:border-brand/50',
                          )}
                        />
                        <DashboardButton
                          icon={<LuSave className="h-3 w-3" />}
                          onClick={() => void handleRenameSave()}
                          disabled={
                            renameSaving || renameValue.trim() === orgName
                          }
                        >
                          {renameSaving ? 'Saving...' : 'Save'}
                        </DashboardButton>
                        <DashboardButton
                          icon={<LuX className="h-3 w-3" />}
                          onClick={handleRenameCancel}
                          disabled={renameSaving}
                        >
                          Cancel
                        </DashboardButton>
                      </div>
                    : <div className="flex items-center justify-between gap-2">
                        <div
                          className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-xs transition-colors"
                          role="button"
                          tabIndex={0}
                          onDoubleClick={handleRenameStart}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') {
                              e.preventDefault()
                              handleRenameStart()
                            }
                          }}
                        >
                          {orgName}
                        </div>
                        <DashboardButton
                          icon={<LuPencil className="h-3 w-3" />}
                          onClick={handleRenameStart}
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

          {isOwner && orgState && (
            <OrgBillingSection
              orgId={orgId}
              billingAccountId={info?.billingAccountId}
              open={openSection === 'billing'}
              onOpenChange={handleSectionOpenChange('billing')}
            />
          )}

          <CollapsibleSection
            title="Identifiers"
            icon={<LuFingerprint className="h-3.5 w-3.5" />}
            open={openSection === 'identifiers'}
            onOpenChange={handleSectionOpenChange('identifiers')}
          >
            <InfoCard>
              <div className="space-y-2">
                <CopyableField label="Organization ID" value={orgId} />
              </div>
            </InfoCard>
          </CollapsibleSection>

          {isOwner && orgState && (
            <OrgActionsSection
              orgId={orgId}
              displayName={orgName}
              spaceCount={spaces.length}
            />
          )}
        </div>
      </div>
    </div>
  )
}
