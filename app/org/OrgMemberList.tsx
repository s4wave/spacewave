import { useCallback, useState } from 'react'
import { LuUserMinus } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import type { OrgMemberInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { ORG_ROLE_OWNER } from './org-constants.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
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
    WebkitUserSelect: 'all',
    userSelect: 'all',
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule7: {
    borderRadius: '3.40282e38px',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '9px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    textTransform: 'uppercase',
  },
  rule8: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule9: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
      <p sx={stylexStyles2.rule1} data-testid="org-members-empty">
        No members yet
      </p>
    )
  }

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1 */ 'space-y-1'
      }
    >
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
  const joinedDate = member.createdAt
    ? new Date(Number(member.createdAt)).toLocaleDateString()
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
    <div sx={stylexStyles2.rule2}>
      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>{getOrgMemberPrimaryLabel(member)}</div>
        {getOrgMemberSecondaryLabel(member) && (
          <div sx={stylexStyles2.rule5}>
            {getOrgMemberSecondaryLabel(member)}
          </div>
        )}
        <div sx={stylexStyles2.rule6}>
          <span
            sx={[
              stylexStyles2.rule7,
              isMemberOwner ? stylexStyles2.rule8 : stylexStyles2.rule9,
            ]}
          >
            {roleLabel}
          </span>
          {joinedDate && <span>Joined {joinedDate}</span>}
        </div>
      </div>
      {isOwner && !isMemberOwner && onRemove && (
        <DashboardButton
          icon={<LuUserMinus sx={stylexStyles.rule1} />}
          onClick={() => void handleRemove()}
          disabled={removing}
          variant="destructive"
        >
          {removing ? 'Removing…' : 'Remove'}
        </DashboardButton>
      )}
    </div>
  )
}
