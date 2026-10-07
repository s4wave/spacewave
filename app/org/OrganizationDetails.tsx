import { useCallback, useId, useState, type ReactNode } from 'react'
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
  LuUserPlus,
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
  OrgInviteInfo,
  OrgMemberInfo,
  SharedObjectMutationPermission,
  WatchOrganizationStateResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
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

/** orgIdentity resolves the display name and role label of the organization. */
function orgIdentity(
  info: WatchOrganizationStateResponse['organization'],
  fallbackOrgName: string | undefined,
  isOwner: boolean,
): { orgName: string; roleLabel: string } {
  return {
    orgName: info?.displayName || fallbackOrgName || 'Organization',
    roleLabel: getRoleLabel(
      info?.role ?? (isOwner ? ORG_ROLE_OWNER : 'org:member'),
    ),
  }
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
    disabledReason: isOwner
      ? ''
      : 'Only organization owners can repair or reinitialize this shared object.',
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
      variant={destructive ? 'destructive' : undefined}
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

/** SectionProps is the open state a CollapsibleSection needs. */
interface SectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** SectionPropsFor returns the open state of the named section. */
type SectionPropsFor = (section: Exclude<OrgOpenSection, null>) => SectionProps

/**
 * tolerateNotFound runs a removal and reports a not-found failure as already
 * done, since the watched org state will drop the entry on its own.
 */
async function tolerateNotFound(
  run: () => Promise<unknown>,
  alreadyDoneMessage: string,
) {
  try {
    await run()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    if (msg.toLowerCase().includes('not found')) {
      toast.info(alreadyDoneMessage, { duration: 3000 })
      return
    }
    throw err
  }
}

/** recoveryToneClasses maps a recovery tone to its card and icon colors. */
const recoveryToneClasses = {
  loading: {
    card: 'border-foreground/8 bg-background-card/30',
    icon: 'bg-foreground/5',
  },
  degraded: {
    card: 'border-warning/20 bg-warning/5',
    icon: 'bg-warning/10',
  },
  closed: {
    card: 'border-destructive/20 bg-destructive/5',
    icon: 'bg-destructive/10',
  },
} as const

type RecoverySummary = ReturnType<typeof getRecoverySummary>

/** RecoveryToneIcon renders the icon for a recovery tone. */
function RecoveryToneIcon({ tone }: { tone: RecoverySummary['tone'] }) {
  if (tone === 'loading') return <Spinner variant="foreground" />
  if (tone === 'degraded') {
    return <LuTriangleAlert className="text-warning size-4" />
  }
  return <LuShieldAlert className="text-destructive size-4" />
}

/** RecoverySummaryCard renders the health summary of the org root object. */
function RecoverySummaryCard({
  summary,
  healthError,
}: {
  summary: RecoverySummary
  healthError?: string
}) {
  const classes = recoveryToneClasses[summary.tone]
  return (
    <div className={cn('rounded-md border px-3 py-2', classes.card)}>
      <div className="flex items-start gap-2">
        <div
          className={cn(
            'flex size-7 shrink-0 items-center justify-center rounded-md',
            classes.icon,
          )}
        >
          <RecoveryToneIcon tone={summary.tone} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-foreground text-xs font-medium">{summary.title}</p>
          <p className="text-foreground-alt/65 mt-1 text-xs">
            {summary.description}
          </p>
          {summary.hint && (
            <p className="text-foreground-alt/60 text-metadata mt-2">
              {summary.hint}
            </p>
          )}
          {healthError && (
            <div className="border-foreground/8 bg-background-card/30 text-foreground-alt/70 micro-label mt-2 rounded-md border px-2 py-1.5 break-words whitespace-pre-wrap">
              {healthError}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/** useRecoveryAction runs repair or reinitialize on the org root object. */
function useRecoveryAction(session: Session | null, sharedObjectId: string) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  const run = useCallback(
    async (kind: 'repair' | 'reinitialize') => {
      if (!session || !sharedObjectId || pending) return
      setPending(true)
      setError('')
      try {
        if (kind === 'repair') {
          const resp =
            await session.spacewave.repairSharedObject(sharedObjectId)
          if (resp.credentialRequired) {
            setError('Shared object recovery requires entity credentials')
          }
        } else {
          await session.spacewave.reinitializeSharedObject(sharedObjectId)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Action failed')
      } finally {
        setPending(false)
      }
    },
    [pending, sharedObjectId, session],
  )

  return { pending, error, run }
}

/** ReinitializeConfirm asks the owner to confirm the destructive reinitialize. */
function ReinitializeConfirm({
  pending,
  onCancel,
  onConfirm,
}: {
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div className="border-destructive/20 bg-destructive/5 mt-3 rounded-md border px-3 py-2">
      <div className="text-destructive/80 micro-caption font-medium tracking-widest uppercase">
        Confirm Reinitialize
      </div>
      <p className="text-foreground-alt/70 mt-1 text-xs">
        Reinitialize is destructive. It rewrites the organization root shared
        object in place on the same shared object id and canonical org route.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <DashboardButton icon={<LuX className="size-3.5" />} onClick={onCancel}>
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={<LuShieldAlert className="size-3.5" />}
          onClick={onConfirm}
          disabled={pending}
          variant="destructive"
        >
          {pending ? 'Reinitializing…' : 'Confirm reinitialize'}
        </DashboardButton>
      </div>
    </div>
  )
}

/** disabledReasonFor returns the reason an action is blocked, or ''. */
function disabledReasonFor(
  allowed: boolean | undefined,
  permission: SharedObjectMutationPermission,
): string {
  return allowed ? '' : (permission.disabledReason ?? '')
}

/** RecoveryRemediation renders the repair and reinitialize controls. */
function RecoveryRemediation({
  session,
  sharedObjectId,
  permission,
}: {
  session: Session | null
  sharedObjectId: string
  permission: SharedObjectMutationPermission
}) {
  const { pending, error, run } = useRecoveryAction(session, sharedObjectId)
  const [confirming, setConfirming] = useState(false)

  return (
    <div className="border-foreground/8 bg-background-card/20 rounded-md border px-3 py-2">
      <div className="text-foreground-alt/45 micro-caption font-medium tracking-widest uppercase">
        Remediation
      </div>
      <p className="text-foreground-alt/65 mt-1 text-xs">
        This works in place on the canonical organization root shared object at
        the current org-owned id.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <RecoveryActionButton
          label={pending ? 'Repairing...' : 'Repair'}
          icon={<LuRefreshCw className="size-3.5" />}
          onClick={() => {
            setConfirming(false)
            void run('repair')
          }}
          disabled={pending}
          disabledReason={disabledReasonFor(permission.canRepair, permission)}
        />
        <RecoveryActionButton
          label="Reinitialize"
          icon={<LuShieldAlert className="size-3.5" />}
          onClick={() => setConfirming(true)}
          disabled={pending}
          disabledReason={disabledReasonFor(
            permission.canReinitialize,
            permission,
          )}
          destructive={true}
        />
      </div>
      {confirming && (
        <ReinitializeConfirm
          pending={pending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false)
            void run('reinitialize')
          }}
        />
      )}
      {error && <p className="text-destructive mt-2 text-xs">{error}</p>}
      <p className="text-foreground-alt/55 mt-2 text-xs">
        Shared object ID: {sharedObjectId}
      </p>
    </div>
  )
}

/** OrgRecoverySection renders the degraded-mode recovery tools. */
function OrgRecoverySection({
  orgId,
  rootState,
  isOwner,
  open,
  onOpenChange,
}: SectionProps & {
  orgId: string
  rootState?: OrganizationRootStateInfo
  isOwner: boolean
}) {
  const session = SessionContext.useContext().value

  return (
    <CollapsibleSection
      title="Recovery"
      icon={<LuCircleAlert className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div className="space-y-3">
          <RecoverySummaryCard
            summary={getRecoverySummary(rootState?.health)}
            healthError={rootState?.health?.error}
          />
          <RecoveryRemediation
            session={session}
            sharedObjectId={rootState?.sharedObjectId || orgId}
            permission={getRecoveryPermission(
              rootState?.mutationPermission,
              isOwner,
            )}
          />
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** OrgMembersSection lists the members and lets owners remove them. */
function OrgMembersSection({
  orgId,
  members,
  isOwner,
  open,
  onOpenChange,
}: SectionProps & {
  orgId: string
  members: OrgMemberInfo[]
  isOwner: boolean
}) {
  const session = SessionContext.useContext().value

  const handleRemoveMember = useCallback(
    async (memberId: string) => {
      if (!session) return
      await tolerateNotFound(
        () => session.spacewave.removeOrgMember(orgId, memberId),
        'Member already removed. Refreshing...',
      )
    },
    [session, orgId],
  )

  return (
    <CollapsibleSection
      title="Members"
      icon={<LuUsers className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
      badge={
        <span className="text-foreground-alt/40 text-xs">{members.length}</span>
      }
    >
      <p className="text-foreground-alt/60 mb-2 text-xs">
        Members are shown by username first. Their account ID stays underneath
        for review or copy.
      </p>
      <OrgMemberList
        members={members}
        isOwner={isOwner}
        onRemove={handleRemoveMember}
      />
    </CollapsibleSection>
  )
}

/** useUsernameInvite sends an org invite to a Spacewave username. */
function useUsernameInvite(session: Session | null, orgId: string) {
  const [username, setUsername] = useState('')
  const [pending, setPending] = useState(false)
  const [sent, setSent] = useState(false)

  const change = useCallback((next: string) => {
    setUsername(next)
    setSent(false)
  }, [])

  const submit = useCallback(async () => {
    const target = username.trim()
    if (!session || !target || pending) return
    setPending(true)
    setSent(false)
    try {
      await session.spacewave.createOrganizationTargetedInvitationByUsername(
        target,
        orgId,
        'org:member',
      )
      setUsername('')
      setSent(true)
    } finally {
      setPending(false)
    }
  }, [session, orgId, username, pending])

  return { username, pending, sent, change, submit }
}

/** usernameInviteLabel returns the invite button label for its state. */
function usernameInviteLabel(pending: boolean, sent: boolean): string {
  if (pending) return 'Sending…'
  return sent ? 'Sent' : 'Invite'
}

/** UsernameInviteForm renders the username field and its invite button. */
function UsernameInviteForm({
  session,
  orgId,
}: {
  session: Session | null
  orgId: string
}) {
  const inputId = useId()
  const invite = useUsernameInvite(session, orgId)

  return (
    <div className="border-foreground/8 bg-background-card/20 mb-3 rounded-md border px-3 py-2">
      <label
        htmlFor={inputId}
        className="text-foreground-alt mb-1.5 block text-xs select-none"
      >
        Spacewave username
      </label>
      <div className="flex gap-2">
        <input
          id={inputId}
          value={invite.username}
          onChange={(e) => invite.change(e.target.value)}
          placeholder="alice"
          className={cn(
            'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 min-w-0 flex-1 rounded-md border px-2 py-1.5 font-mono text-xs transition-colors outline-none',
            'focus:border-brand/50',
          )}
        />
        <DashboardButton
          icon={<LuUserPlus className="size-3" />}
          onClick={() => void invite.submit()}
          disabled={invite.pending || !invite.username.trim() || !session}
        >
          {usernameInviteLabel(invite.pending, invite.sent)}
        </DashboardButton>
      </div>
    </div>
  )
}

/** OrgInvitesSection lists invites and lets owners create and revoke them. */
function OrgInvitesSection({
  orgId,
  invites,
  open,
  onOpenChange,
}: SectionProps & { orgId: string; invites: OrgInviteInfo[] }) {
  const session = SessionContext.useContext().value
  const [creatingInvite, setCreatingInvite] = useState(false)

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
      await tolerateNotFound(
        () => session.spacewave.revokeOrgInvite(orgId, inviteId),
        'Invite already revoked. Refreshing...',
      )
    },
    [session, orgId],
  )

  return (
    <CollapsibleSection
      title="Invites"
      icon={<LuLink className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
      badge={
        invites.length > 0 ? (
          <span className="text-foreground-alt/40 text-xs">
            {invites.length}
          </span>
        ) : undefined
      }
      headerActions={
        <button
          type="button"
          onClick={() => void handleCreateInvite()}
          disabled={creatingInvite}
          aria-label="Create invite"
          title="Create invite"
          className="text-foreground-alt hover:text-foreground flex size-4 items-center justify-center transition-colors disabled:cursor-not-allowed disabled:opacity-50"
        >
          <LuPlus className="size-3.5" />
        </button>
      }
    >
      <UsernameInviteForm session={session} orgId={orgId} />
      <OrgInviteSection invites={invites} onRevoke={handleRevokeInvite} />
    </CollapsibleSection>
  )
}

/** focusInput focuses the input when it mounts. */
function focusInput(node: HTMLInputElement | null) {
  node?.focus()
}

/** useOrgRename edits and saves the organization display name. */
function useOrgRename(session: Session | null, orgId: string, orgName: string) {
  const [value, setValue] = useState('')
  const [renaming, setRenaming] = useState(false)
  const [saving, setSaving] = useState(false)

  const start = useCallback(() => {
    setValue(orgName)
    setRenaming(true)
  }, [orgName])

  const cancel = useCallback(() => {
    setRenaming(false)
    setValue('')
  }, [])

  const save = useCallback(async () => {
    if (!session || saving || value.trim() === orgName) return
    setSaving(true)
    try {
      await session.spacewave.updateOrganization(orgId, value.trim())
      setRenaming(false)
    } finally {
      setSaving(false)
    }
  }, [session, orgId, value, orgName, saving])

  return { value, renaming, saving, setValue, start, cancel, save }
}

type OrgRename = ReturnType<typeof useOrgRename>

/** OrgRenameEditor renders the display name input with save and cancel. */
function OrgRenameEditor({
  inputId,
  orgName,
  rename,
}: {
  inputId: string
  orgName: string
  rename: OrgRename
}) {
  return (
    <div className="flex items-center gap-2">
      <input
        ref={focusInput}
        id={inputId}
        type="text"
        value={rename.value}
        onChange={(e) => rename.setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter') void rename.save()
          if (e.key === 'Escape') rename.cancel()
        }}
        className={cn(
          'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-2 py-1 text-xs transition-colors outline-none',
          'focus:border-brand/50',
        )}
      />
      <DashboardButton
        icon={<LuSave className="size-3" />}
        onClick={() => void rename.save()}
        disabled={rename.saving || rename.value.trim() === orgName}
      >
        {rename.saving ? 'Saving…' : 'Save'}
      </DashboardButton>
      <DashboardButton
        icon={<LuX className="size-3" />}
        onClick={rename.cancel}
        disabled={rename.saving}
      >
        Cancel
      </DashboardButton>
    </div>
  )
}

/** OrgRenameDisplay renders the display name with its edit affordances. */
function OrgRenameDisplay({
  orgName,
  onStart,
}: {
  orgName: string
  onStart: () => void
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <button
        type="button"
        className="text-foreground hover:text-foreground-alt min-w-0 flex-1 cursor-text text-left text-xs transition-colors"
        onDoubleClick={onStart}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onStart()
          }
        }}
      >
        {orgName}
      </button>
      <DashboardButton icon={<LuPencil className="size-3" />} onClick={onStart}>
        Edit
      </DashboardButton>
    </div>
  )
}

/** OrgSettingsSection lets owners rename the organization. */
function OrgSettingsSection({
  orgId,
  orgName,
  open,
  onOpenChange,
}: SectionProps & { orgId: string; orgName: string }) {
  const renameInputId = useId()
  const session = SessionContext.useContext().value
  const rename = useOrgRename(session, orgId, orgName)

  return (
    <CollapsibleSection
      title="Settings"
      icon={<LuSettings className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div className="space-y-2">
          <div>
            <label
              htmlFor={renameInputId}
              className="text-foreground-alt mb-1 block text-xs select-none"
            >
              Display Name
            </label>
            {rename.renaming ? (
              <OrgRenameEditor
                inputId={renameInputId}
                orgName={orgName}
                rename={rename}
              />
            ) : (
              <OrgRenameDisplay orgName={orgName} onStart={rename.start} />
            )}
          </div>
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** OrgOwnerSections renders the invite, settings, and billing sections. */
function OrgOwnerSections({
  orgId,
  orgName,
  billingAccountId,
  invites,
  sectionProps,
}: {
  orgId: string
  orgName: string
  billingAccountId?: string
  invites: OrgInviteInfo[]
  sectionProps: SectionPropsFor
}) {
  return (
    <>
      <OrgInvitesSection
        orgId={orgId}
        invites={invites}
        {...sectionProps('invites')}
      />
      <OrgSettingsSection
        orgId={orgId}
        orgName={orgName}
        {...sectionProps('settings')}
      />
      <OrgBillingSection
        orgId={orgId}
        billingAccountId={billingAccountId}
        {...sectionProps('billing')}
      />
    </>
  )
}

/** OrgHeader renders the organization title, leave action, and close button. */
function OrgHeader({
  orgId,
  orgName,
  roleLabel,
  canLeave,
  onCloseClick,
}: {
  orgId: string
  orgName: string
  roleLabel: string
  canLeave: boolean
  onCloseClick?: () => void
}) {
  const session = SessionContext.useContext().value
  const navigateSession = useSessionNavigate()

  const handleLeave = useCallback(async () => {
    if (!session) return
    await session.spacewave.leaveOrganization(orgId)
    navigateSession({ path: '' })
  }, [session, orgId, navigateSession])

  return (
    <div className="border-foreground/8 flex min-h-9 shrink-0 items-center justify-between gap-3 border-b px-4 py-2">
      <div className="text-foreground flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold select-none">
        <div className="bg-brand/10 text-brand flex size-5 shrink-0 items-center justify-center rounded">
          <LuBuilding2 className="size-3" />
        </div>
        <span className="min-w-0 truncate tracking-tight">{orgName}</span>
        <span className="text-foreground-alt/50 text-xs font-normal">
          {roleLabel}
        </span>
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
        {canLeave && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuLogOut className="size-4" />}
                variant="destructive"
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
                icon={<LuX className="size-4" />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/** OrgLoading renders the placeholder shown until the org state arrives. */
function OrgLoading() {
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
  const ns = useStateNamespace(['org-details'])
  const [openSection, setOpenSection] = useStateAtom<OrgOpenSection>(
    ns,
    'open-section',
    degraded ? 'recovery' : 'members',
  )

  const sectionProps = useCallback<SectionPropsFor>(
    (section) => ({
      open: openSection === section,
      onOpenChange: (open) => setOpenSection(open ? section : null),
    }),
    [openSection, setOpenSection],
  )

  const info = orgState?.organization
  const { orgName, roleLabel } = orgIdentity(info, fallbackOrgName, isOwner)

  if (!orgState && !degraded) {
    return <OrgLoading />
  }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <OrgHeader
        orgId={orgId}
        orgName={orgName}
        roleLabel={roleLabel}
        canLeave={!isOwner && !!info}
        onCloseClick={onCloseClick}
      />

      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div className="space-y-3">
          {degraded && (
            <OrgRecoverySection
              orgId={orgId}
              rootState={orgState?.rootState}
              isOwner={isOwner}
              {...sectionProps('recovery')}
            />
          )}
          {orgState && (
            <OrgMembersSection
              orgId={orgId}
              members={orgState.members ?? []}
              isOwner={isOwner}
              {...sectionProps('members')}
            />
          )}
          {isOwner && orgState && (
            <OrgOwnerSections
              orgId={orgId}
              orgName={orgName}
              billingAccountId={info?.billingAccountId}
              invites={orgState.invites ?? []}
              sectionProps={sectionProps}
            />
          )}

          <CollapsibleSection
            title="Identifiers"
            icon={<LuFingerprint className="size-3.5" />}
            {...sectionProps('identifiers')}
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
              spaceCount={orgState.spaces?.length ?? 0}
            />
          )}
        </div>
      </div>
    </div>
  )
}
