import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { useCallback } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'

import {
  type SessionSyncSummary,
  useSessionSyncStatus,
  useSessionSyncSummary,
} from './SessionSyncStatusContext.js'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import {
  radii as themeRadii,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles7 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles6 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    maxHeight: `calc(${themeLayout['--spacing']} * 56)`,
    overflowY: 'auto',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: { minWidth: 0, flex: 1 },
  rule9: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule13: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule14: {
    cursor: 'pointer',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule15: { marginTop: themeLayout['--spacing'], overflowWrap: 'break-word' },
  rule16: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule17: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule18: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule19: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule20: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule21: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule23: { textAlign: 'right', overflowWrap: 'break-word' },
  rule24: { color: themeColors['--color-destructive'] },
  rule25: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: { color: themeColors['--color-brand'] },
  rule2: { color: themeColors['--color-destructive'] },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * -1)`,
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-brand'],
  },
})

// SessionSyncStatusButton registers the session sync-status bottom-bar item.
// The button follows the sync summary; only the open popover follows the
// transfer rates.
export function SessionSyncStatusButton() {
  const status = useSessionSyncSummary()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            sx={[
              sx,
              !!status.active && stylexStyles3.rule1,
              !!status.error && stylexStyles3.rule2,
            ]}
            aria-label={status.ariaLabel}
            data-testid="session-sync-status-button"
          >
            <SessionSyncStatusGlyph status={status} />
          </BottomBarItem>
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          sideOffset={6}
          onEscapeKeyDown={onClick}
          onPointerDownOutside={onClick}
          variant="status"
        >
          <SessionSyncStatusPopover />
        </PopoverContent>
      </Popover>
    ),
    [status],
  )

  return (
    <BottomBarLevel
      id="session-sync-status"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

function SessionSyncStatusGlyph({ status }: { status: SessionSyncSummary }) {
  if (status.loading || status.active) {
    return <Spinner size="sm" />
  }
  if (status.error) {
    return <LuCircleAlert sx={stylexStyles.rule1} aria-hidden="true" />
  }
  const Icon = status.local ? LuHardDrive : LuCloud
  return (
    <span sx={stylexStyles4.rule1} aria-hidden="true">
      <Icon sx={stylexStyles2.rule1} />
      <LuCircleCheck sx={stylexStyles.rule2} />
    </span>
  )
}

function SessionSyncStatusPopover() {
  const status = useSessionSyncStatus()
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3 p-3'
      }
      data-testid="session-sync-status-popover"
    >
      <div sx={stylexStyles4.rule2}>
        <div
          sx={[
            stylexStyles4.rule3,
            !!status.error && stylexStyles4.rule4,
            !!status.active && stylexStyles4.rule5,
            status.visualState === 'synced' && stylexStyles4.rule6,
            !!status.loading && stylexStyles4.rule7,
          ]}
        >
          <SessionSyncStatusGlyph status={status} />
        </div>
        <div sx={stylexStyles4.rule8}>
          <div sx={stylexStyles4.rule9}>{status.summaryLabel}</div>
          <div sx={stylexStyles4.rule10}>{status.detailLabel}</div>
        </div>
      </div>

      <div sx={stylexStyles4.rule11}>
        <SyncMetric label="Upload" value={status.uploadRateLabel} />
        <SyncMetric label="Download" value={status.downloadRateLabel} />
        {status.local ? (
          <>
            <SyncMetric label="Sent to peers" value={status.peerUploadLabel} />
            <SyncMetric
              label="Received from peers"
              value={status.peerDownloadLabel}
            />
          </>
        ) : (
          <>
            <SyncMetric
              label="Uploading now"
              value={status.activeUploadLabel}
            />
            <SyncMetric
              label="Upload queued"
              value={status.pendingUploadLabel}
            />
            <SyncMetric
              label="Download queued"
              value={status.pendingDownloadLabel}
            />
          </>
        )}
      </div>

      {status.localCopies.length > 0 && (
        <div sx={stylexStyles5.rule1}>
          {status.localCopies.map((copy) => (
            <div sx={nativeChildSpacing.s3} key={copy.id}>
              <SyncRow label={copy.name} value={copy.status} />
              <div sx={stylexStyles4.rule12}>{copy.detail}</div>
              {copy.error && (
                <details sx={stylexStyles4.rule13}>
                  <summary sx={stylexStyles4.rule14}>Copy details</summary>
                  <p sx={stylexStyles4.rule15}>{copy.error}</p>
                </details>
              )}
            </div>
          ))}
        </div>
      )}

      {!status.local && (
        <details sx={stylexStyles7.rule1}>
          <summary sx={[nativeChildSpacingNextNext.s1_5, stylexStyles4.rule16]}>
            Storage diagnostics
          </summary>
          <SyncRow
            sx={nativeChildSpacingNextNext.s1_5}
            label="Ranges"
            value={status.packRangeLabel}
          />
          <SyncRow
            sx={nativeChildSpacingNextNext.s1_5}
            label="Index tail"
            value={status.packIndexTailLabel}
          />
          <SyncRow
            sx={nativeChildSpacingNextNext.s1_5}
            label="Lookup"
            value={status.packLookupLabel}
          />
          <SyncRow
            sx={nativeChildSpacingNextNext.s1_5}
            label="Index cache"
            value={status.packIndexCacheLabel}
          />
        </details>
      )}

      {status.peers.length > 0 && (
        <details sx={stylexStyles6.rule1}>
          <summary sx={[nativeChildSpacingNext.s2, stylexStyles4.rule16]}>
            Linked Sessions
          </summary>
          {status.peers.map((peer) => (
            <div sx={nativeChildSpacingNext.s2} key={peer.id}>
              <SyncRow label={peer.name} value={peer.state} />
              <div sx={stylexStyles4.rule17}>{peer.traffic}</div>
            </div>
          ))}
        </details>
      )}

      <div sx={stylexStyles7.rule1}>
        <SyncRow
          sx={nativeChildSpacingNextNext.s1_5}
          label="Transport"
          value={status.transportLabel}
        />
        <SyncRow
          sx={nativeChildSpacingNextNext.s1_5}
          label="P2P"
          value={status.p2pLabel}
        />
        <SyncRow
          sx={nativeChildSpacingNextNext.s1_5}
          label="Activity"
          value={status.lastActivityLabel}
        />
        {status.lastError && (
          <SyncRow
            sx={nativeChildSpacingNextNext.s1_5}
            label="Last error"
            value={status.lastError}
            tone="error"
          />
        )}
      </div>
    </div>
  )
}

function SyncMetric({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles4.rule18}>
      <div sx={stylexStyles4.rule19}>{label}</div>
      <div sx={stylexStyles4.rule20}>{value}</div>
    </div>
  )
}

function SyncRow({
  sx,
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'error'
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles4.rule21, sx]}>
      <span sx={stylexStyles4.rule22}>{label}</span>
      <span
        sx={[
          stylexStyles4.rule23,
          tone === 'error' ? stylexStyles4.rule24 : stylexStyles4.rule25,
        ]}
      >
        {value}
      </span>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
