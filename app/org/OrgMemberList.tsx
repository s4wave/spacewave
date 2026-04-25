import { useCallback, useState } from 'react'
import { LuUserMinus } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import type { OrgMemberInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { ORG_ROLE_OWNER } from './org-constants.js'

function getOrgMemberPrimaryLabel(member: OrgMemberInfo): string {
  return member.entityId || member.subjectId || 'Unknown member'
}

function getOrgMemberSecondaryLabel(member: OrgMemberInfo): string {
  if (member.entityId && member.subjectId) return member.subjectId
  return ''
}

// OrgMemberList renders the list of org members with optional remove action.
export function OrgMemberList(props: {
  members: OrgMemberInfo[]
  isOwner: boolean
  onRemove?: (memberId: string) => Promise<void>
}) {
  if (props.members.length === 0) {
    return (
      <p
        className="text-foreground-alt/50 py-2 text-xs"
        data-testid="org-members-empty"
      >
        No members yet
      </p>
    )
  }

  return (
    <div className="space-y-1">
      {props.members.map((member) => (
        <OrgMemberRow
          key={member.id}
          member={member}
          isOwner={props.isOwner}
          onRemove={props.onRemove}
        />
      ))}
    </div>
  )
}

function OrgMemberRow(props: {
  member: OrgMemberInfo
  isOwner: boolean
  onRemove?: (memberId: string) => Promise<void>
}) {
  const { member, isOwner, onRemove } = props
  const [removing, setRemoving] = useState(false)
  const isMemberOwner = member.roleId === ORG_ROLE_OWNER
  const roleLabel = isMemberOwner ? 'Owner' : 'Member'
  const joinedDate =
    member.createdAt ?
      new Date(Number(member.createdAt)).toLocaleDateString()
    : ''

  const handleRemove = useCallback(async () => {
    if (!onRemove || removing) return
    setRemoving(true)
    try {
      await onRemove(member.subjectId ?? '')
    } finally {
      setRemoving(false)
    }
  }, [onRemove, removing, member.subjectId])

  return (
    <div className="flex items-center justify-between gap-2 py-1">
      <div className="min-w-0 flex-1">
        <div className="text-foreground truncate text-xs font-medium">
          {getOrgMemberPrimaryLabel(member)}
        </div>
        {getOrgMemberSecondaryLabel(member) && (
          <div className="text-foreground-alt/50 truncate font-mono text-[10px] select-all">
            {getOrgMemberSecondaryLabel(member)}
          </div>
        )}
        <div className="text-foreground-alt/50 flex items-center gap-2 text-xs">
          <span
            className={cn(
              'rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase',
              isMemberOwner ?
                'bg-brand/15 text-brand'
              : 'bg-foreground/10 text-foreground-alt/70',
            )}
          >
            {roleLabel}
          </span>
          {joinedDate && <span>Joined {joinedDate}</span>}
        </div>
      </div>
      {isOwner && !isMemberOwner && onRemove && (
        <DashboardButton
          icon={<LuUserMinus className="h-3 w-3" />}
          onClick={() => void handleRemove()}
          disabled={removing}
          className="text-destructive hover:bg-destructive/10"
        >
          {removing ? 'Removing...' : 'Remove'}
        </DashboardButton>
      )}
    </div>
  )
}
