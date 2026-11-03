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
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule6: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
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
          <ul
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 divide-y'
            }
          >
            {sync.localCopies.map((copy) => (
              <li key={copy.id} sx={[false, edgeSpacing.s2]}>
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
          <ul
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 divide-y'
            }
          >
            {sync.peers.map((peer) => (
              <li key={peer.id} sx={[false, edgeSpacing.s2]}>
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
    paddingTop: { default: 'calc(var(--spacing) * 2)', ':first-child': 0 },
    paddingBottom: { default: 'calc(var(--spacing) * 2)', ':last-child': 0 },
  },
})
