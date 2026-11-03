/* eslint-disable react-doctor/async-await-in-loop */
import { useCallback, useMemo, useReducer } from 'react'
import {
  LuCheck,
  LuShield,
  LuTrash2,
  LuUsers,
  LuX,
} from '@s4wave/web/images/icons.js'

import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'
import type { SOInvite } from '@s4wave/core/sobject/sobject.pb.js'
import type { MailboxEntryInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  SpaceControl,
  type SpaceParticipantInfo,
} from '@s4wave/sdk/space/space.pb.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    marginBottom: themeLayout['--spacing'],
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule3: {
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule4: { padding: themeLayout['--spacing'] },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule8: { minWidth: 0, flex: 1 },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule10: {
    marginLeft: themeLayout['--spacing'],
    fontFamily: 'var(--font-sans)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule11: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule12: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule14: {
    cursor: 'pointer',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule15: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule17: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule18: {
    cursor: 'pointer',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-green-500'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule19: {
    cursor: 'pointer',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const roleLabels: Record<number, string> = {
  [SOParticipantRole.SOParticipantRole_OWNER]: 'Owner',
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

/**
 * useSpaceMemberActions runs the remove, revoke, and process-request actions
 * of the members panel and tracks which row is busy.
 */
function useSpaceMemberActions(
  session: Session | null | undefined,
  spaceId: string | undefined,
) {
  const [state, dispatch] = useReducer(reducer, {
    removingMember: undefined,
    revokingInvite: undefined,
    processingEntry: undefined,
    error: undefined,
  })

  const handleRemove = useCallback(
    async (member: SpaceParticipantInfo) => {
      if (!session || !spaceId) return
      dispatch({ type: 'removing', memberId: getParticipantKey(member) })
      try {
        let awaitingGroup = false
        if (session.spacewave && member.accountId) {
          const resp = await session.spacewave.removeSpaceMember(
            spaceId,
            member.accountId,
          )
          awaitingGroup = !!resp.awaitingGroup
        } else if (member.peerIds?.length) {
          const resp = await session.removeSpaceParticipants(
            spaceId,
            member.peerIds,
          )
          awaitingGroup = !!resp.awaitingGroup
        }
        dispatch({ type: 'done' })
        if (awaitingGroup) {
          toast.info('Asked the group', {
            description: `${getParticipantPrimaryLabel(member)} leaves once enough members agree.`,
          })
        }
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

  return { state, handleRemove, handleRevoke, handleProcessEntry }
}

/** InvitesSection lists the active invites with their revoke actions. */
function InvitesSection({
  invites,
  bordered,
  canRevoke,
  revokingInvite,
  onRevoke,
}: {
  invites: SOInvite[]
  bordered: boolean
  canRevoke: boolean
  revokingInvite: string | undefined
  onRevoke: (inviteId: string) => void
}) {
  return (
    <div sx={[!!bordered && stylexStyles2.rule1]}>
      <div sx={stylexStyles2.rule2}>Invites</div>
      <div>
        {invites.map((inv) => (
          <InviteRow
            sx={nativeChildSpacing.s1}
            key={inv.inviteId}
            invite={inv}
            canRevoke={canRevoke}
            revoking={revokingInvite === inv.inviteId}
            onRevoke={() => onRevoke(inv.inviteId ?? '')}
          />
        ))}
      </div>
    </div>
  )
}

/** PendingRequestsSection lists the pending mailbox requests with accept and reject actions. */
function PendingRequestsSection({
  entries,
  bordered,
  processingEntry,
  onProcess,
}: {
  entries: MailboxEntryInfo[]
  bordered: boolean
  processingEntry: bigint | undefined
  onProcess: (entryId: bigint, accept: boolean) => void
}) {
  return (
    <div sx={[!!bordered && stylexStyles2.rule1]}>
      <div sx={stylexStyles2.rule2}>Pending Requests</div>
      {entries.length > 0 ? (
        <div>
          {entries.map((entry) => (
            <PendingRequestRow
              sx={nativeChildSpacing.s1}
              key={String(entry.id)}
              entry={entry}
              processing={processingEntry === entry.id}
              onAccept={() => onProcess(entry.id ?? BigInt(0), true)}
              onReject={() => onProcess(entry.id ?? BigInt(0), false)}
            />
          ))}
        </div>
      ) : (
        <div sx={stylexStyles2.rule3} data-testid="pending-request-empty">
          No pending requests yet
        </div>
      )}
    </div>
  )
}

// SpaceMembersPanelProps configures the members panel.
export interface SpaceMembersPanelProps {
  // Tightens the surrounding card padding for a narrow container.
  compact?: boolean
}

// SpaceMembersPanel displays active members and invites for a space.
export function SpaceMembersPanel({ compact = false }: SpaceMembersPanelProps) {
  const session = useResourceValue(SessionContext.useContext())
  const { spaceId, spaceSharingState } = SpaceContainerContext.useContext()

  const { state, handleRemove, handleRevoke, handleProcessEntry } =
    useSpaceMemberActions(session, spaceId)

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
  // Under group control a voter asks the group to remove a member.
  const canRemove =
    spaceSharingState?.control === SpaceControl.SpaceControl_GROUP
      ? !!spaceSharingState.canVote
      : canManage

  const empty =
    !!spaceSharingState &&
    participantInfo.length === 0 &&
    invites.length === 0 &&
    mailboxEntries.length === 0
  const showPendingRequests = mailboxEntries.length > 0 || invites.length > 0

  return (
    <InfoCard compact={compact}>
      {!spaceSharingState && (
        <div sx={stylexStyles2.rule4}>
          <LoadingInline label="Loading sharing state" tone="muted" size="sm" />
        </div>
      )}

      {empty && (
        <div sx={stylexStyles2.rule5}>
          <LuUsers sx={stylexStyles.rule1} />
          <span>No users added yet</span>
        </div>
      )}

      {participantInfo.length > 0 && (
        <div data-testid="space-members-panel">
          {participantInfo.map((member) => (
            <MemberRow
              sx={nativeChildSpacing.s1}
              key={getParticipantKey(member)}
              memberId={getParticipantKey(member)}
              primaryLabel={getParticipantPrimaryLabel(member)}
              secondaryLabel={getParticipantSecondaryLabel(member)}
              role={member.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN}
              isSelf={member.isSelf ?? false}
              deviceCount={member.peerIds?.length ?? 0}
              canRemove={canRemove && !member.isSelf}
              removing={state.removingMember === getParticipantKey(member)}
              onRemove={() => void handleRemove(member)}
            />
          ))}
        </div>
      )}

      {invites.length > 0 && (
        <InvitesSection
          invites={invites}
          bordered={participantInfo.length > 0}
          canRevoke={canManage}
          revokingInvite={state.revokingInvite}
          onRevoke={(inviteId) => void handleRevoke(inviteId)}
        />
      )}

      {showPendingRequests && (
        <PendingRequestsSection
          entries={mailboxEntries}
          bordered={participantInfo.length > 0 || invites.length > 0}
          processingEntry={state.processingEntry}
          onProcess={(entryId, accept) =>
            void handleProcessEntry(entryId, accept)
          }
        />
      )}

      {state.error && <p sx={stylexStyles2.rule6}>{state.error}</p>}
    </InfoCard>
  )
}

function MemberRow(
  props: {
    memberId: string
    primaryLabel: string
    secondaryLabel: string
    role: SOParticipantRole
    isSelf: boolean
    deviceCount: number
    canRemove: boolean
    removing: boolean
    onRemove: () => void
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  return (
    <div
      sx={[stylexStyles2.rule7, props.sx]}
      data-testid="space-member-row"
      data-member-id={props.memberId}
    >
      <LuShield sx={stylexStyles.rule2} />
      <div sx={stylexStyles2.rule8}>
        <div sx={stylexStyles2.rule9} data-testid="space-member-label">
          {props.primaryLabel}
          {props.isSelf && <span sx={stylexStyles2.rule10}>(you)</span>}
        </div>
        {props.secondaryLabel && (
          <div sx={stylexStyles2.rule11}>{props.secondaryLabel}</div>
        )}
      </div>
      {props.deviceCount > 1 && (
        <span sx={stylexStyles2.rule12} data-testid="space-member-device-count">
          {props.deviceCount} devices
        </span>
      )}
      <span sx={stylexStyles2.rule13}>{roleName(props.role)}</span>
      {props.canRemove && (
        <button
          type="button"
          aria-label="Remove member"
          onClick={props.onRemove}
          disabled={props.removing}
          sx={stylexStyles2.rule14}
          data-testid="space-member-remove"
        >
          {props.removing ? (
            <Spinner size="sm" />
          ) : (
            <LuX sx={stylexStyles.rule3} />
          )}
        </button>
      )}
    </div>
  )
}

function InviteRow(
  props: {
    invite: SOInvite
    canRevoke: boolean
    revoking: boolean
    onRevoke: () => void
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  const inv = props.invite
  const usageText = inv.maxUses
    ? `${inv.uses ?? 0}/${inv.maxUses} uses`
    : `${inv.uses ?? 0} uses`

  return (
    <div sx={[stylexStyles2.rule7, props.sx]}>
      <span sx={stylexStyles2.rule15}>
        {truncatePeerId(inv.inviteId ?? '')}
      </span>
      <span sx={stylexStyles2.rule16}>
        {roleName(inv.role ?? SOParticipantRole.SOParticipantRole_UNKNOWN)}
      </span>
      <span sx={stylexStyles2.rule17}>{usageText}</span>
      {props.canRevoke && (
        <button
          type="button"
          aria-label="Revoke invite"
          onClick={props.onRevoke}
          disabled={props.revoking}
          sx={stylexStyles2.rule14}
        >
          {props.revoking ? (
            <Spinner size="sm" />
          ) : (
            <LuTrash2 sx={stylexStyles.rule3} />
          )}
        </button>
      )}
    </div>
  )
}

function PendingRequestRow(
  props: {
    entry: MailboxEntryInfo
    processing: boolean
    onAccept: () => void
    onReject: () => void
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  const primaryLabel = getMailboxEntryPrimaryLabel(props.entry)
  const secondaryLabel = getMailboxEntrySecondaryLabel(props.entry)
  const inviteLabel = getMailboxEntryInviteLabel(props.entry)

  return (
    <div sx={[stylexStyles2.rule7, props.sx]}>
      <div sx={stylexStyles2.rule8}>
        <div sx={stylexStyles2.rule9} data-testid="pending-request-label">
          {primaryLabel}
        </div>
        {secondaryLabel && (
          <div sx={stylexStyles2.rule11}>{secondaryLabel}</div>
        )}
      </div>
      {inviteLabel && <span sx={stylexStyles2.rule16}>{inviteLabel}</span>}
      {props.processing ? (
        <Spinner size="sm" variant="subtle" />
      ) : (
        <>
          <button
            type="button"
            aria-label="Accept join request"
            onClick={props.onAccept}
            sx={stylexStyles2.rule18}
          >
            <LuCheck sx={stylexStyles.rule3} />
          </button>
          <button
            type="button"
            aria-label="Reject join request"
            onClick={props.onReject}
            sx={stylexStyles2.rule19}
          >
            <LuX sx={stylexStyles.rule3} />
          </button>
        </>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
