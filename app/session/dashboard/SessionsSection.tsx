import { useCallback, useMemo, useState } from 'react'
import { LuCloud, LuLogOut, LuSmartphone, LuUnlink } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import {
  AccountEscalationIntentKind,
  AccountSessionKind,
  type AccountSession,
} from '@s4wave/sdk/account/account.pb.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

import {
  AuthConfirmDialog,
  buildEntityCredential,
  type AuthCredential,
} from './AuthConfirmDialog.js'

export interface SessionsSectionProps {
  account: Resource<Account>
  isLocal: boolean
  retainStepUp?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onLinkDeviceClick?: () => void
}

// SessionsSection renders the provider-account attached session list for both
// local and cloud providers.
export function SessionsSection({
  account,
  isLocal,
  retainStepUp = false,
  open,
  onOpenChange,
  onLinkDeviceClick,
}: SessionsSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const mountedAccount = useResourceValue(account)
  const navigateSession = useSessionNavigate()
  const sessionsResource = useStreamingResource(
    account,
    useCallback(
      (value: NonNullable<Account>, signal: AbortSignal) =>
        value.watchSessions({}, signal),
      [],
    ),
    [],
  )
  const rows: AccountSession[] = sessionsResource.value?.sessions ?? []
  const loading = sessionsResource.loading
  const [pendingPeerId, setPendingPeerId] = useState<string | null>(null)
  const [revokeRow, setRevokeRow] = useState<AccountSession | null>(null)
  const isOpen = open ?? true
  const handleOpenChange = onOpenChange ?? (() => {})

  const handleLinkDeviceClick = useCallback(() => {
    if (onLinkDeviceClick) {
      onLinkDeviceClick()
      return
    }
    navigateSession({ path: 'setup/link-device' })
  }, [navigateSession, onLinkDeviceClick])

  const handleRowAction = useCallback(
    async (row: AccountSession) => {
      const peerId = row.peerId ?? ''
      if (!peerId || row.currentSession) return
      const isLocalRow =
        row.kind ===
        AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION
      if (!isLocalRow) {
        setRevokeRow(row)
        return
      }
      const label = row.label || peerId
      if (!window.confirm(`Are you sure you want to unlink ${label}?`)) {
        return
      }

      setPendingPeerId(peerId)
      try {
        if (!session) return
        await session.unlinkDevice(peerId)
      } catch {
        // Watched snapshot convergence is authoritative; errors can be surfaced later.
      } finally {
        setPendingPeerId(null)
      }
    },
    [session],
  )

  const handleConfirmRevoke = useCallback(
    async (credential: AuthCredential) => {
      const peerId = revokeRow?.peerId ?? ''
      if (!mountedAccount || !peerId) return

      setPendingPeerId(peerId)
      try {
        await mountedAccount.revokeSession({
          sessionPeerId: peerId,
          credential: buildEntityCredential(credential),
        })
        setRevokeRow(null)
      } finally {
        setPendingPeerId(null)
      }
    },
    [mountedAccount, revokeRow],
  )

  const badge = useMemo(() => {
    if (rows.length === 0) return undefined
    return (
      <span className="text-foreground-alt/50 text-[0.55rem]">
        {rows.length}
      </span>
    )
  }, [rows.length])

  return (
    <CollapsibleSection
      title="Sessions"
      icon={<LuCloud className="h-3.5 w-3.5" />}
      open={isOpen}
      onOpenChange={handleOpenChange}
      badge={badge}
    >
      <div className="space-y-2">
        {loading && (
          <p className="text-foreground-alt text-xs">Loading sessions...</p>
        )}
        {!loading && rows.length === 0 && (
          <div className="flex items-center justify-between py-1">
            <p className="text-foreground-alt text-xs">
              {isLocal ? 'No linked sessions yet.' : 'No other sessions found.'}
            </p>
            {isLocal && (
              <button
                onClick={handleLinkDeviceClick}
                className="text-brand hover:text-brand/80 text-xs font-medium transition-colors"
              >
                Link My Device
              </button>
            )}
          </div>
        )}
        {!loading && rows.length > 0 && (
          <div className="space-y-2">
            {rows.map((row) => (
              <SessionRow
                key={row.peerId}
                row={row}
                pending={pendingPeerId === (row.peerId ?? '')}
                onAction={handleRowAction}
              />
            ))}
            {isLocal && (
              <div className="border-foreground/10 border-t pt-2">
                <button
                  onClick={handleLinkDeviceClick}
                  className="text-brand hover:text-brand/80 text-xs font-medium transition-colors"
                >
                  Link Another Device
                </button>
              </div>
            )}
          </div>
        )}
      </div>
      <AuthConfirmDialog
        open={!!revokeRow}
        onOpenChange={(next) => {
          if (!next) {
            setRevokeRow(null)
          }
        }}
        title="Sign Out Session"
        description={`Sign out ${revokeRow?.label || revokeRow?.peerId || 'this session'} from Spacewave Cloud.`}
        confirmLabel="Sign Out"
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_REVOKE_SESSION,
          title: 'Sign Out Session',
          description: `Sign out ${revokeRow?.label || revokeRow?.peerId || 'this session'} from Spacewave Cloud.`,
          targetLabel: revokeRow?.label,
          targetPeerId: revokeRow?.peerId,
        }}
        onConfirm={handleConfirmRevoke}
        account={account}
        retainAfterClose={retainStepUp}
      />
    </CollapsibleSection>
  )
}

interface SessionRowProps {
  row: AccountSession
  pending: boolean
  onAction: (row: AccountSession) => Promise<void>
}

function SessionRow({ row, pending, onAction }: SessionRowProps) {
  const kind =
    row.kind ??
    AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_UNSPECIFIED
  const isLocalRow =
    kind ===
    AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION
  const details = [row.clientName, row.os, row.location]
    .filter(Boolean)
    .join(' · ')
  const createdAt = row.createdAt ?? null
  const lastSeenAt = row.lastSeenAt ?? null
  const status =
    row.currentSession ? 'Current session'
    : isLocalRow ?
      createdAt ? `Paired ${createdAt.toLocaleDateString()}`
      : 'Linked device'
    : lastSeenAt ? `Last seen ${lastSeenAt.toLocaleDateString()}`
    : createdAt ? `Created ${createdAt.toLocaleDateString()}`
    : 'Cloud session'

  const actionLabel = isLocalRow ? 'Unlink session' : 'Log out session'
  const label = row.label || row.peerId || 'Session'

  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <div className="relative shrink-0">
          {isLocalRow ?
            <LuSmartphone className="text-foreground-alt h-3.5 w-3.5" />
          : <LuCloud className="text-foreground-alt h-3.5 w-3.5" />}
          {row.currentSession && (
            <span className="bg-success absolute -top-0.5 -right-0.5 h-1.5 w-1.5 rounded-full" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="text-foreground truncate text-xs font-medium">
              {label}
            </p>
            {row.currentSession && (
              <span className="border-success/20 bg-success/10 text-success rounded-full border px-1.5 py-0.5 text-[0.55rem] font-medium">
                This device
              </span>
            )}
            {!row.currentSession && isLocalRow && (
              <span className="border-foreground/10 bg-foreground/5 text-foreground-alt rounded-full border px-1.5 py-0.5 text-[0.55rem] font-medium">
                Linked
              </span>
            )}
          </div>
          <p className="text-foreground-alt text-xs">
            {details ? `${status} · ${details}` : status}
          </p>
        </div>
      </div>
      {!row.currentSession && (
        <button
          onClick={() => void onAction(row)}
          disabled={pending}
          className={cn(
            'text-foreground-alt hover:text-destructive flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors',
            pending && 'cursor-not-allowed opacity-50',
          )}
          title={actionLabel}
        >
          {isLocalRow ?
            <LuUnlink className="h-3 w-3" />
          : <LuLogOut className="h-3 w-3" />}
        </button>
      )}
    </div>
  )
}
