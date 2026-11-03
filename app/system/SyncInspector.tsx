import { utilityState as themeUtilityState } from '@s4wave/web/style/tokens.stylex.js'
import type { SessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'baseline',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'space-between',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
  },
  rule6: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule7: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
})

// SyncInspector shows every reading of the session sync watcher: status,
// throughput, local copies, peers, and pack engine counters.
export function SyncInspector({ sync }: { sync: SessionSyncStatusView }) {
  return (
    <div sx={stylexStyles.rule1}>
      <InspectorSection title="Status" description={sync.detailLabel}>
        <Facts
          facts={[
            { label: 'State', value: sync.summaryLabel },
            { label: 'Transport', value: sync.transportLabel },
            { label: 'Peer-to-peer', value: sync.p2pLabel },
            { label: 'Last activity', value: sync.lastActivityLabel },
            { label: 'Last error', value: sync.lastError, tone: 'error' },
          ]}
        />
      </InspectorSection>

      <InspectorSection title="Throughput">
        <Facts
          facts={[
            { label: 'Upload rate', value: sync.uploadRateLabel, mono: true },
            {
              label: 'Download rate',
              value: sync.downloadRateLabel,
              mono: true,
            },
            {
              label: 'Upload waiting',
              value: sync.pendingUploadLabel,
              mono: true,
            },
            {
              label: 'Uploading now',
              value: sync.activeUploadLabel,
              mono: true,
            },
            {
              label: 'Download waiting',
              value: sync.pendingDownloadLabel,
              mono: true,
            },
            { label: 'Sent to peers', value: sync.peerUploadLabel, mono: true },
            {
              label: 'Received from peers',
              value: sync.peerDownloadLabel,
              mono: true,
            },
          ]}
        />
      </InspectorSection>

      <InspectorSection
        title="Local copies"
        description="Stores on this device that hold a copy of your data."
      >
        {sync.localCopies.length === 0 ? (
          <p sx={stylexStyles.rule2}>No local copies reported.</p>
        ) : (
          <ul>
            {sync.localCopies.map((copy) => (
              <li
                key={copy.id}
                sx={[[false, edgeSpacing.s2], nativeDividers.t6]}
              >
                <div sx={stylexStyles.rule3}>
                  <span sx={stylexStyles.rule4}>{copy.name}</span>
                  <span sx={stylexStyles.rule5}>{copy.status}</span>
                </div>
                <p sx={stylexStyles.rule6}>{copy.detail}</p>
                {copy.error && <p sx={stylexStyles.rule7}>{copy.error}</p>}
              </li>
            ))}
          </ul>
        )}
      </InspectorSection>

      <InspectorSection
        title="Sync peers"
        description="Devices exchanging blocks with this session."
      >
        {sync.peers.length === 0 ? (
          <p sx={stylexStyles.rule2}>No peers are exchanging data right now.</p>
        ) : (
          <ul>
            {sync.peers.map((peer) => (
              <li
                key={peer.id}
                sx={[[false, edgeSpacing.s2], nativeDividers.t6]}
              >
                <div sx={stylexStyles.rule3}>
                  <span sx={stylexStyles.rule4}>{peer.name}</span>
                  <span sx={stylexStyles.rule5}>{peer.state}</span>
                </div>
                <p sx={stylexStyles.rule6}>{peer.traffic}</p>
              </li>
            ))}
          </ul>
        )}
      </InspectorSection>

      <InspectorSection
        title="Pack engine"
        description="Counters from the block pack reader that serves synced data."
      >
        <Facts
          facts={[
            { label: 'Range reads', value: sync.packRangeLabel, mono: true },
            {
              label: 'Index tail reads',
              value: sync.packIndexTailLabel,
              mono: true,
            },
            { label: 'Lookups', value: sync.packLookupLabel, mono: true },
            {
              label: 'Index cache',
              value: sync.packIndexCacheLabel,
              mono: true,
            },
          ]}
        />
      </InspectorSection>
    </div>
  )
}

// edgeSpacing keeps first and last row padding at the row owner.
const edgeSpacing = stylex.create({
  s2: {
    paddingBottom: { default: 'calc(var(--spacing) * 2)', ':last-child': 0 },
    paddingTop: { default: 'calc(var(--spacing) * 2)', ':first-child': 0 },
  },
})

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t6: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})
