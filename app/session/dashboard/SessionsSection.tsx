import { useCallback, useState } from 'react'
import {
  LuCloud,
  LuLink,
  LuLogOut,
  LuSmartphone,
  LuUnlink,
} from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
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
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface SessionsSectionProps {
  account: Resource<Account>
  isLocal: boolean
  retainStepUp?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onLinkDeviceClick?: () => void
}

// useSessionActions unlinks local sessions and signs out cloud sessions. A
// cloud session opens a revoke step-up for the row, which handleConfirmRevoke
// completes.
function useSessionActions(account: Resource<Account>) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const mountedAccount = useResourceValue(account)
  const [pendingPeerId, setPendingPeerId] = useState<string | null>(null)
  const [revokeRow, setRevokeRow] = useState<AccountSession | null>(null)

  const handleRowAction = async (row: AccountSession) => {
    const peerId = row.peerId ?? ''
    if (!peerId || row.currentSession) return
    if (!isLocalSession(row)) {
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
  }

  const handleConfirmRevoke = async (credential: AuthCredential) => {
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
  }

  return {
    pendingPeerId,
    revokeRow,
    clearRevokeRow: () => setRevokeRow(null),
    handleRowAction,
    handleConfirmRevoke,
  }
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
  const actions = useSessionActions(account)
  const isOpen = open ?? true
  const handleOpenChange = onOpenChange ?? (() => {})

  const handleLinkDeviceClick = () => {
    if (onLinkDeviceClick) {
      onLinkDeviceClick()
      return
    }
    navigateSession({ path: 'setup/link-device' })
  }

  return (
    <CollapsibleSection
      title="Sessions"
      icon={<LuCloud sx={stylexStyles.rule1} />}
      open={isOpen}
      onOpenChange={handleOpenChange}
      badge={
        rows.length > 0 ? (
          <span className="text-foreground-alt/50 text-xs">{rows.length}</span>
        ) : undefined
      }
    >
      <div className="space-y-2">
        {sessionsResource.loading && (
          <p className="text-foreground-alt text-xs">Loading sessions…</p>
        )}
        {!sessionsResource.loading && rows.length === 0 && (
          <div className="flex items-center justify-between py-1">
            <p className="text-foreground-alt text-xs">
              {isLocal ? 'No linked sessions yet.' : 'No other sessions found.'}
            </p>
            {isLocal && (
              <button
                type="button"
                onClick={handleLinkDeviceClick}
                className="text-brand hover:text-brand/80 text-xs font-medium transition-colors"
              >
                Link My Device
              </button>
            )}
          </div>
        )}
        {!sessionsResource.loading && rows.length > 0 && (
          <div className="space-y-2">
            {rows.map((row) => (
              <SessionRow
                key={row.peerId}
                row={row}
                pending={actions.pendingPeerId === (row.peerId ?? '')}
                onAction={actions.handleRowAction}
              />
            ))}
            {isLocal && <LinkDeviceCard onClick={handleLinkDeviceClick} />}
          </div>
        )}
      </div>
      {!isLocal && (
        <RevokeSessionDialog
          account={account}
          row={actions.revokeRow}
          retainStepUp={retainStepUp}
          onClose={actions.clearRevokeRow}
          onConfirm={actions.handleConfirmRevoke}
        />
      )}
    </CollapsibleSection>
  )
}

// LinkDeviceCard renders the action that links another device.
function LinkDeviceCard({ onClick }: { onClick: () => void }) {
  return (
    <div className="border-foreground/10 border-t pt-2">
      <button
        type="button"
        onClick={onClick}
        sx={[stylexStyles2.rule1, groupMarker2]}
      >
        <div className="bg-foreground/10 group-hover:bg-brand/10 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors">
          <LuLink sx={stylexStyles.rule2} />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-foreground text-xs font-medium select-none">
            Link Another Device
          </span>
          <span className="text-foreground-alt text-xs select-none">
            Connect another device to sync your data peer-to-peer.
          </span>
        </div>
      </button>
    </div>
  )
}

// RevokeSessionDialog confirms signing a cloud session out. The dialog is open
// while row is set.
function RevokeSessionDialog({
  account,
  row,
  retainStepUp,
  onClose,
  onConfirm,
}: {
  account: Resource<Account>
  row: AccountSession | null
  retainStepUp: boolean
  onClose: () => void
  onConfirm: (credential: AuthCredential) => Promise<void>
}) {
  const description = `Sign out ${row?.label || row?.peerId || 'this session'} from Spacewave Cloud.`

  return (
    <AuthConfirmDialog
      open={!!row}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title="Sign Out Session"
      description={description}
      confirmLabel="Sign Out"
      intent={{
        kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_REVOKE_SESSION,
        title: 'Sign Out Session',
        description,
        targetLabel: row?.label,
        targetPeerId: row?.peerId,
      }}
      onConfirm={onConfirm}
      account={account}
      retainAfterClose={retainStepUp}
    />
  )
}

// isLocalSession reports whether row is a session linked through the local
// provider.
function isLocalSession(row: AccountSession): boolean {
  return (
    row.kind ===
    AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION
  )
}

// describeSessionStatus summarizes when a session was last seen, paired, or
// created.
function describeSessionStatus(row: AccountSession): string {
  if (row.currentSession) return 'Current session'
  if (row.lastSeenAt) return `Last seen ${row.lastSeenAt.toLocaleDateString()}`
  if (isLocalSession(row)) {
    return row.createdAt
      ? `Paired ${row.createdAt.toLocaleDateString()}`
      : 'Linked device'
  }
  return row.createdAt
    ? `Created ${row.createdAt.toLocaleDateString()}`
    : 'Cloud session'
}

interface SessionRowProps {
  row: AccountSession
  pending: boolean
  onAction: (row: AccountSession) => Promise<void>
}

function SessionRow({ row, pending, onAction }: SessionRowProps) {
  const isLocalRow = isLocalSession(row)
  // The local provider stamps deviceType "linked", which the Linked badge
  // already conveys; cloud sessions carry a real platform ("web", "desktop").
  const platform = row.deviceType === 'linked' ? '' : row.deviceType
  const details = [platform, row.clientName, row.os, row.location]
    .filter(Boolean)
    .join(' · ')
  const status = describeSessionStatus(row)

  const actionLabel = isLocalRow ? 'Unlink session' : 'Log out session'
  const label = row.label || row.peerId || 'Session'

  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <div className="relative shrink-0">
          {isLocalRow ? (
            <LuSmartphone sx={stylexStyles.rule3} />
          ) : (
            <LuCloud sx={stylexStyles.rule3} />
          )}
          {row.currentSession && (
            <span className="bg-success absolute -top-0.5 -right-0.5 size-1.5 rounded-full" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="text-foreground truncate text-xs font-medium">
              {label}
            </p>
            {/* The local session's default label is already "This device";
                skip the badge there so the row shows one, not two. */}
            {row.currentSession && label !== 'This device' && (
              <span className="border-success/20 bg-success/10 text-success rounded-full border px-1.5 py-0.5 text-xs font-medium">
                This device
              </span>
            )}
            {!row.currentSession && isLocalRow && (
              <span className="border-foreground/10 bg-foreground/5 text-foreground-alt rounded-full border px-1.5 py-0.5 text-xs font-medium">
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
          type="button"
          onClick={() => void onAction(row)}
          disabled={pending}
          className={cn(
            'text-foreground-alt hover:text-destructive flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors',
            pending && 'cursor-not-allowed opacity-50',
          )}
          title={actionLabel}
        >
          {isLocalRow ? (
            <LuUnlink sx={stylexStyles.rule4} />
          ) : (
            <LuLogOut sx={stylexStyles.rule4} />
          )}
        </button>
      )}
    </div>
  )
}
