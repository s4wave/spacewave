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

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingBlock: themeLayout['--spacing'],
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule5: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: { position: 'relative', flexShrink: 0 },
  rule13: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * -.5)`,
    right: `calc(${themeLayout['--spacing']} * -.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-success'],
  },
  rule14: { minWidth: 0, flex: 1 },
  rule15: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule17: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-success']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-success'],
  },
  rule18: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule19: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
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
  rule20: { cursor: 'not-allowed', opacity: 0.5 },
})

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
          <span sx={stylexStyles3.rule1}>{rows.length}</span>
        ) : undefined
      }
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
        {sessionsResource.loading && (
          <p sx={stylexStyles3.rule2}>Loading sessions…</p>
        )}
        {!sessionsResource.loading && rows.length === 0 && (
          <div sx={stylexStyles3.rule3}>
            <p sx={stylexStyles3.rule2}>
              {isLocal ? 'No linked sessions yet.' : 'No other sessions found.'}
            </p>
            {isLocal && (
              <button
                type="button"
                onClick={handleLinkDeviceClick}
                sx={stylexStyles3.rule4}
              >
                Link My Device
              </button>
            )}
          </div>
        )}
        {!sessionsResource.loading && rows.length > 0 && (
          <div>
            {rows.map((row) => (
              <SessionRow
                sx={nativeChildSpacing.s2}
                key={row.peerId}
                row={row}
                pending={actions.pendingPeerId === (row.peerId ?? '')}
                onAction={actions.handleRowAction}
              />
            ))}
            {isLocal && (
              <LinkDeviceCard
                sx={nativeChildSpacing.s2}
                onClick={handleLinkDeviceClick}
              />
            )}
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
function LinkDeviceCard({
  sx,
  onClick,
}: { onClick: () => void } & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles3.rule5, sx]}>
      <button
        type="button"
        onClick={onClick}
        sx={[stylexStyles2.rule1, groupMarker2]}
      >
        <div sx={stylexStyles3.rule6}>
          <LuLink sx={stylexStyles.rule2} />
        </div>
        <div sx={stylexStyles3.rule7}>
          <span sx={stylexStyles3.rule8}>Link Another Device</span>
          <span sx={stylexStyles3.rule9}>
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

function SessionRow({
  sx,
  row,
  pending,
  onAction,
}: SessionRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
    <div sx={[stylexStyles3.rule10, sx]}>
      <div sx={stylexStyles3.rule11}>
        <div sx={stylexStyles3.rule12}>
          {isLocalRow ? (
            <LuSmartphone sx={stylexStyles.rule3} />
          ) : (
            <LuCloud sx={stylexStyles.rule3} />
          )}
          {row.currentSession && <span sx={stylexStyles3.rule13} />}
        </div>
        <div sx={stylexStyles3.rule14}>
          <div sx={stylexStyles3.rule15}>
            <p sx={stylexStyles3.rule16}>{label}</p>
            {/* The local session's default label is already "This device";
                skip the badge there so the row shows one, not two. */}
            {row.currentSession && label !== 'This device' && (
              <span sx={stylexStyles3.rule17}>This device</span>
            )}
            {!row.currentSession && isLocalRow && (
              <span sx={stylexStyles3.rule18}>Linked</span>
            )}
          </div>
          <p sx={stylexStyles3.rule2}>
            {details ? `${status} · ${details}` : status}
          </p>
        </div>
      </div>
      {!row.currentSession && (
        <button
          type="button"
          onClick={() => void onAction(row)}
          disabled={pending}
          sx={[stylexStyles3.rule19, !!pending && stylexStyles3.rule20]}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
