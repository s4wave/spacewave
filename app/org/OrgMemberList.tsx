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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: { flex: 1, minWidth: 0 },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'all',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    userSelect: 'all',
    whiteSpace: 'nowrap',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
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
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
    <div>
      {props.members.map((member) => (
        <OrgMemberRow
          sx={nativeChildSpacing.s1}
          key={member.id}
          member={member}
          isOwner={props.isOwner}
          onRemove={props.onRemove}
        />
      ))}
    </div>
  )
}

function OrgMemberRow(
  props: {
    member: OrgMemberInfo
    isOwner: boolean
    onRemove?: (memberId: string) => Promise<void>
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
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
    <div sx={[stylexStyles2.rule2, props.sx]}>
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
