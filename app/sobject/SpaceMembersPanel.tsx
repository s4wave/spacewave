import { useCallback, useMemo, useReducer } from 'react'
import { LuCheck, LuShield, LuTrash2, LuUsers, LuX } from 'react-icons/lu'

import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'
import type { SOInvite } from '@s4wave/core/sobject/sobject.pb.js'
import type { MailboxEntryInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { SpaceParticipantInfo } from '@s4wave/sdk/space/space.pb.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { cn } from '@s4wave/web/style/utils.js'
import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

const roleLabels: Record<number, string> = {
  [SOParticipantRole.SOParticipantRole_OWNER]: 'Owner',
  [SOParticipantRole.SOParticipantRole_VALIDATOR]: 'Validator',
  [SOParticipantRole.SOParticipantRole_WRITER]: 'Writer',
  [SOParticipantRole.SOParticipantRole_READER]: 'Reader',
}

function roleName(role: SOParticipantRole): string {
  return roleLabels[role] ?? 'Unknown'
}

interface PanelState {
  removingMember: string | undefined
  revokingInvite: string | undefined
  processingEntry: bigint | undefined
  error: string | undefined
}

type PanelAction =
  | { type: 'removing'; memberId: string }
  | { type: 'revoking'; inviteId: string }
  | { type: 'processing'; entryId: bigint }
  | { type: 'done' }
  | { type: 'error'; message: string }

function reducer(state: PanelState, action: PanelAction): PanelState {
  switch (action.type) {
    case 'removing':
      return { ...state, removingMember: action.memberId, error: undefined }
    case 'revoking':
      return { ...state, revokingInvite: action.inviteId, error: undefined }
    case 'processing':
      return { ...state, processingEntry: action.entryId, error: undefined }
    case 'done':
      return {
        removingMember: undefined,
        revokingInvite: undefined,
        processingEntry: undefined,
        error: undefined,
      }
    case 'error':
      return {
        ...state,
        removingMember: undefined,
        revokingInvite: undefined,
        processingEntry: undefined,
        error: action.message,
      }
  }
}

function getParticipantKey(member: SpaceParticipantInfo): string {
  if (member.accountId) return member.accountId
  if (member.peerIds && member.peerIds.length !== 0) return member.peerIds[0]
  return 'unknown'
}

function getParticipantPrimaryLabel(member: SpaceParticipantInfo): string {
  return (
    member.entityId ||
    member.accountId ||
    truncatePeerId(member.peerIds?.[0] ?? '')
  )
}

function getParticipantSecondaryLabel(member: SpaceParticipantInfo): string {
  if (member.entityId && member.accountId) return member.accountId
  if (member.accountId && member.peerIds && member.peerIds.length !== 0) {
    return truncatePeerId(member.peerIds[0] ?? '')
  }
  if (member.peerIds && member.peerIds.length !== 0) {
    return truncatePeerId(member.peerIds[0] ?? '')
  }
  return ''
}

function getMailboxEntryPrimaryLabel(entry: MailboxEntryInfo): string {
  return entry.entityId || entry.accountId || truncatePeerId(entry.peerId ?? '')
}

function getMailboxEntrySecondaryLabel(entry: MailboxEntryInfo): string {
  if (entry.entityId && entry.accountId) return entry.accountId
  if (entry.accountId && entry.peerId) return truncatePeerId(entry.peerId)
  if (entry.peerId) return truncatePeerId(entry.peerId)
  return ''
}

function getMailboxEntryInviteLabel(entry: MailboxEntryInfo): string {
  if (!entry.inviteId) return ''
  return `via ${truncatePeerId(entry.inviteId)}`
}

// SpaceMembersPanel displays active members and invites for a space.
export function SpaceMembersPanel() {
  const session = useResourceValue(SessionContext.useContext())
  const { spaceId, spaceSharingState } = SpaceContainerContext.useContext()

  const [state, dispatch] = useReducer(reducer, {
    removingMember: undefined,
    revokingInvite: undefined,
    processingEntry: undefined,
    error: undefined,
  })

  const participantInfo = useMemo(
    () => spaceSharingState?.participantInfo ?? [],
    [spaceSharingState?.participantInfo],
  )
  const invites = useMemo(
    () => (spaceSharingState?.invites ?? []).filter((inv) => !inv.revoked),
    [spaceSharingState?.invites],
  )
  const mailboxEntries = useMemo(
    () =>
      (spaceSharingState?.mailboxEntries ?? []).filter(
        (entry) => entry.status === 'pending',
      ),
    [spaceSharingState?.mailboxEntries],
  )
  const canManage = spaceSharingState?.canManage ?? false

  const handleRemove = useCallback(
    async (member: SpaceParticipantInfo) => {
      if (!session || !spaceId) return
      dispatch({ type: 'removing', memberId: getParticipantKey(member) })
      try {
        if (session.spacewave && member.accountId) {
          await session.spacewave.removeSpaceMember(spaceId, member.accountId)
        } else {
          for (const peerId of member.peerIds ?? []) {
            await session.removeSpaceParticipant(spaceId, peerId)
          }
        }
        dispatch({ type: 'done' })
      } catch (err) {
        dispatch({
          type: 'error',
          message:
            err instanceof Error ? err.message : 'Failed to remove member',
        })
      }
    },
    [session, spaceId],
  )

  const handleRevoke = useCallback(
    async (inviteId: string) => {
      if (!session || !spaceId) return
      dispatch({ type: 'revoking', inviteId })
      try {
        await session.revokeSpaceInvite(spaceId, inviteId)
        dispatch({ type: 'done' })
      } catch (err) {
        dispatch({
          type: 'error',
          message:
            err instanceof Error ? err.message : 'Failed to revoke invite',
        })
      }
    },
    [session, spaceId],
  )

  const handleProcessEntry = useCallback(
    async (entryId: bigint, accept: boolean) => {
      if (!session || !spaceId) return
      dispatch({ type: 'processing', entryId })
      try {
        await session.spacewave.processMailboxEntry(spaceId, entryId, accept)
        dispatch({ type: 'done' })
      } catch (err) {
        dispatch({
          type: 'error',
          message:
            err instanceof Error ? err.message : 'Failed to process request',
        })
      }
    },
    [session, spaceId],
  )

  const empty =
    !!spaceSharingState &&
    participantInfo.length === 0 &&
    invites.length === 0 &&
    mailboxEntries.length === 0
  const showPendingRequests = mailboxEntries.length > 0 || invites.length > 0

  return (
    <InfoCard>
      {!spaceSharingState && (
        <div className="px-1 py-1">
          <LoadingInline label="Loading sharing state" tone="muted" size="sm" />
        </div>
      )}

      {empty && (
        <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
          <LuUsers className="h-3.5 w-3.5 shrink-0" />
          <span>No users added yet</span>
        </div>
      )}

      {participantInfo.length > 0 && (
        <div className="space-y-1" data-testid="space-members-panel">
          {participantInfo.map((member) => (
            <MemberRow
              key={getParticipantKey(member)}
              memberId={getParticipantKey(member)}
              primaryLabel={getParticipantPrimaryLabel(member)}
              secondaryLabel={getParticipantSecondaryLabel(member)}
              role={member.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN}
              isSelf={member.isSelf ?? false}
              deviceCount={member.peerIds?.length ?? 0}
              canRemove={canManage && !member.isSelf}
              removing={state.removingMember === getParticipantKey(member)}
              onRemove={() => void handleRemove(member)}
            />
          ))}
        </div>
      )}

      {invites.length > 0 && (
        <div
          className={cn(
            participantInfo.length > 0 &&
              'border-foreground/6 mt-2 border-t pt-2',
          )}
        >
          <div className="text-foreground-alt/50 mb-1 text-[0.6rem] font-medium tracking-wider uppercase">
            Invites
          </div>
          <div className="space-y-1">
            {invites.map((inv) => (
              <InviteRow
                key={inv.inviteId}
                invite={inv}
                canRevoke={canManage}
                revoking={state.revokingInvite === inv.inviteId}
                onRevoke={() => void handleRevoke(inv.inviteId ?? '')}
              />
            ))}
          </div>
        </div>
      )}

      {showPendingRequests && (
        <div
          className={cn(
            (participantInfo.length > 0 || invites.length > 0) &&
              'border-foreground/6 mt-2 border-t pt-2',
          )}
        >
          <div className="text-foreground-alt/50 mb-1 text-[0.6rem] font-medium tracking-wider uppercase">
            Pending Requests
          </div>
          {mailboxEntries.length > 0 ?
            <div className="space-y-1">
              {mailboxEntries.map((entry) => (
                <PendingRequestRow
                  key={String(entry.id)}
                  entry={entry}
                  processing={state.processingEntry === entry.id}
                  onAccept={() =>
                    void handleProcessEntry(entry.id ?? BigInt(0), true)
                  }
                  onReject={() =>
                    void handleProcessEntry(entry.id ?? BigInt(0), false)
                  }
                />
              ))}
            </div>
          : <div
              className="text-foreground-alt/40 px-1 py-0.5 text-xs"
              data-testid="pending-request-empty"
            >
              No pending requests yet
            </div>
          }
        </div>
      )}

      {state.error && (
        <p className="text-destructive mt-1 text-xs">{state.error}</p>
      )}
    </InfoCard>
  )
}

function MemberRow(props: {
  memberId: string
  primaryLabel: string
  secondaryLabel: string
  role: SOParticipantRole
  isSelf: boolean
  deviceCount: number
  canRemove: boolean
  removing: boolean
  onRemove: () => void
}) {
  return (
    <div
      className="flex items-center gap-2 py-0.5"
      data-testid="space-member-row"
      data-member-id={props.memberId}
    >
      <LuShield className="text-foreground-alt/40 h-3 w-3 shrink-0" />
      <div className="min-w-0 flex-1">
        <div
          className="text-foreground truncate text-xs font-medium"
          data-testid="space-member-label"
        >
          {props.primaryLabel}
          {props.isSelf && (
            <span className="text-foreground-alt/50 ml-1 font-sans">(you)</span>
          )}
        </div>
        {props.secondaryLabel && (
          <div className="text-foreground-alt/50 truncate font-mono text-[10px]">
            {props.secondaryLabel}
          </div>
        )}
      </div>
      {props.deviceCount > 1 && (
        <span
          className="text-foreground-alt/40 border-foreground/10 rounded-full border px-1.5 py-0.5 text-[0.55rem]"
          data-testid="space-member-device-count"
        >
          {props.deviceCount} devices
        </span>
      )}
      <span className="text-foreground-alt/50 text-[0.6rem]">
        {roleName(props.role)}
      </span>
      {props.canRemove && (
        <button
          onClick={props.onRemove}
          disabled={props.removing}
          className="text-foreground-alt/40 hover:text-destructive cursor-pointer transition-colors disabled:opacity-50"
          data-testid="space-member-remove"
        >
          {props.removing ?
            <Spinner size="sm" />
          : <LuX className="h-3 w-3" />}
        </button>
      )}
    </div>
  )
}

function InviteRow(props: {
  invite: SOInvite
  canRevoke: boolean
  revoking: boolean
  onRevoke: () => void
}) {
  const inv = props.invite
  const usageText =
    inv.maxUses ?
      `${inv.uses ?? 0}/${inv.maxUses} uses`
    : `${inv.uses ?? 0} uses`

  return (
    <div className="flex items-center gap-2 py-0.5">
      <span className="text-foreground-alt/60 min-w-0 flex-1 truncate font-mono text-xs">
        {truncatePeerId(inv.inviteId ?? '')}
      </span>
      <span className="text-foreground-alt/40 text-[0.6rem]">
        {roleName(inv.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN)}
      </span>
      <span className="text-foreground-alt/30 text-[0.6rem]">{usageText}</span>
      {props.canRevoke && (
        <button
          onClick={props.onRevoke}
          disabled={props.revoking}
          className="text-foreground-alt/40 hover:text-destructive cursor-pointer transition-colors disabled:opacity-50"
        >
          {props.revoking ?
            <Spinner size="sm" />
          : <LuTrash2 className="h-3 w-3" />}
        </button>
      )}
    </div>
  )
}

function PendingRequestRow(props: {
  entry: MailboxEntryInfo
  processing: boolean
  onAccept: () => void
  onReject: () => void
}) {
  const primaryLabel = getMailboxEntryPrimaryLabel(props.entry)
  const secondaryLabel = getMailboxEntrySecondaryLabel(props.entry)
  const inviteLabel = getMailboxEntryInviteLabel(props.entry)

  return (
    <div className="flex items-center gap-2 py-0.5">
      <div className="min-w-0 flex-1">
        <div
          className="text-foreground truncate text-xs font-medium"
          data-testid="pending-request-label"
        >
          {primaryLabel}
        </div>
        {secondaryLabel && (
          <div className="text-foreground-alt/50 truncate font-mono text-[10px]">
            {secondaryLabel}
          </div>
        )}
      </div>
      {inviteLabel && (
        <span className="text-foreground-alt/40 text-[0.6rem]">
          {inviteLabel}
        </span>
      )}
      {props.processing ?
        <Spinner size="sm" className="text-foreground-alt/40" />
      : <>
          <button
            onClick={props.onAccept}
            className="text-foreground-alt/40 cursor-pointer transition-colors hover:text-green-500"
          >
            <LuCheck className="h-3 w-3" />
          </button>
          <button
            onClick={props.onReject}
            className="text-foreground-alt/40 hover:text-destructive cursor-pointer transition-colors"
          >
            <LuX className="h-3 w-3" />
          </button>
        </>
      }
    </div>
  )
}
