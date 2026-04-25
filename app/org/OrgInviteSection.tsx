import { useCallback, useEffect, useState } from 'react'
import { LuClipboard, LuLink, LuTrash2 } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import type { OrgInviteInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// OrgInviteSection renders the invite list. Create is handled by the parent
// section header.
export function OrgInviteSection(props: {
  invites: OrgInviteInfo[]
  onRevoke: (inviteId: string) => Promise<void>
}) {
  return (
    <div className="space-y-1">
      {props.invites.length === 0 && (
        <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
          <LuLink className="h-3.5 w-3.5 shrink-0" />
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

  const usesLabel =
    invite.maxUses ?
      `${invite.uses ?? 0}/${invite.maxUses} uses`
    : `${invite.uses ?? 0} uses`

  return (
    <div className="flex items-center justify-between gap-2 py-1">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <LuLink className="text-foreground-alt/50 h-3 w-3 shrink-0" />
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
              className={cn('h-3 w-3', copied && 'text-green-500')}
            />
          }
          onClick={() => void handleCopy()}
        >
          {copied ? 'Copied' : 'Copy'}
        </DashboardButton>
        <DashboardButton
          icon={<LuTrash2 className="h-3 w-3" />}
          onClick={() => void handleRevoke()}
          disabled={revoking}
          className="text-destructive hover:bg-destructive/10"
        >
          Revoke
        </DashboardButton>
      </div>
    </div>
  )
}
