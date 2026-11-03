import { useCallback, useEffect, useState } from 'react'
import { LuClipboard, LuLink, LuTrash2 } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import type { OrgInviteInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule7: { gap: themeLayout['--spacing'], display: 'flex', flexShrink: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: { color: themeColors['--color-green-500'] },
})

// OrgInviteSection renders the invite list. Create is handled by the parent
// section header.
export function OrgInviteSection(props: {
  invites: OrgInviteInfo[]
  onRevoke: (inviteId: string) => Promise<void>
}) {
  return (
    <div>
      {props.invites.length === 0 && (
        <div sx={[nativeChildSpacing.s1, stylexStyles2.rule1]}>
          <LuLink sx={stylexStyles.rule1} />
          <span>No active invites</span>
        </div>
      )}
      {props.invites.map((invite) => (
        <InviteRow
          sx={nativeChildSpacing.s1}
          key={invite.id}
          invite={invite}
          onRevoke={props.onRevoke}
        />
      ))}
    </div>
  )
}

function InviteRow(
  props: {
    invite: OrgInviteInfo
    onRevoke: (inviteId: string) => Promise<void>
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  const { invite, onRevoke } = props
  const [copied, setCopied] = useState(false)
  const [revoking, setRevoking] = useState(false)

  useEffect(() => {
    if (!copied) return
    const id = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(id)
  }, [copied])

  const handleCopy = useCallback(async () => {
    const token = invite.token ?? ''
    await navigator.clipboard.writeText(token)
    setCopied(true)
  }, [invite.token])

  const handleRevoke = useCallback(async () => {
    if (revoking) return
    setRevoking(true)
    try {
      await onRevoke(invite.id ?? '')
    } finally {
      setRevoking(false)
    }
  }, [onRevoke, invite.id, revoking])

  const usesLabel = invite.maxUses
    ? `${invite.uses ?? 0}/${invite.maxUses} uses`
    : `${invite.uses ?? 0} uses`

  return (
    <div sx={[stylexStyles2.rule2, props.sx]}>
      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>
          <LuLink sx={stylexStyles.rule2} />
          <span sx={stylexStyles2.rule5}>{invite.token}</span>
        </div>
        <div sx={stylexStyles2.rule6}>{usesLabel}</div>
      </div>
      <div sx={stylexStyles2.rule7}>
        <DashboardButton
          icon={
            <LuClipboard
              sx={[stylexStyles.rule3, !!copied && stylexStyles.rule4]}
            />
          }
          onClick={() => void handleCopy()}
        >
          {copied ? 'Copied' : 'Copy'}
        </DashboardButton>
        <DashboardButton
          icon={<LuTrash2 sx={stylexStyles.rule3} />}
          onClick={() => void handleRevoke()}
          disabled={revoking}
          variant="destructive"
        >
          Revoke
        </DashboardButton>
      </div>
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
