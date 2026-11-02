import { useCallback, useEffect, useState } from 'react'
import { LuClipboard, LuLink, LuTrash2 } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import type { OrgInviteInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
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
    <div className="space-y-1">
      {props.invites.length === 0 && (
        <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
          <LuLink sx={stylexStyles.rule1} />
          <span>No active invites</span>
        </div>
      )}
      {props.invites.map((invite) => (
        <InviteRow key={invite.id} invite={invite} onRevoke={props.onRevoke} />
      ))}
    </div>
  )
}

function InviteRow(props: {
  invite: OrgInviteInfo
  onRevoke: (inviteId: string) => Promise<void>
}) {
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
    <div className="flex items-center justify-between gap-2 py-1">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <LuLink sx={stylexStyles.rule2} />
          <span className="text-foreground truncate font-mono text-xs">
            {invite.token}
          </span>
        </div>
        <div className="text-foreground-alt/50 ml-5 text-xs">{usesLabel}</div>
      </div>
      <div className="flex shrink-0 gap-1">
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
