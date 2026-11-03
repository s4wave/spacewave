import type { SOSyncPeer } from '@s4wave/core/sobject/sobject.pb.js'
import type { SpaceSharingState } from '@s4wave/sdk/space/space.pb.js'

import { CopyableField } from '@s4wave/web/ui/CopyableField.js'

import { deviceName } from './device-name.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    columnGap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'space-between',
  },
  rule4: { color: themeColors['--color-foreground'] },
  rule5: { color: themeColors['--color-foreground-alt'] },
  rule6: { color: themeColors['--color-warning'] },
  rule7: { color: themeColors['--color-warning'], overflowWrap: 'break-word' },
})

// SyncPeerList shows this device's sync stream with each other device that
// has synced the Space since this device started, and whether it still runs.
// Peer IDs show only with the technical details.
export function SyncPeerList({
  sx,
  state,
  peers,
  showDetails,
}: {
  state: SpaceSharingState
  peers: SOSyncPeer[]
  showDetails: boolean
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={sx}>
      <h4 sx={[nativeChildSpacingNextNext.s1, stylexStyles.rule1]}>
        Other devices
      </h4>
      {peers.length === 0 ? (
        <p sx={[nativeChildSpacingNextNext.s1, stylexStyles.rule2]}>
          No other device has synced since this device started.
        </p>
      ) : (
        <ul sx={nativeChildSpacingNextNext.s1} aria-label="Other devices">
          {peers.map((peer) => (
            <li
              key={peer.peerId}
              sx={[nativeChildSpacingNext.s1, stylexStyles2.rule1]}
            >
              <div sx={[nativeChildSpacing.s0_5, stylexStyles.rule3]}>
                <span sx={stylexStyles.rule4}>
                  {deviceName(state, peer.peerId ?? '', 'Unknown device')}
                </span>
                <span
                  sx={[
                    peer.connected ? stylexStyles.rule5 : stylexStyles.rule6,
                  ]}
                >
                  {peerStatus(peer)}
                </span>
              </div>
              {!peer.connected && peer.error && (
                <p sx={[nativeChildSpacing.s0_5, stylexStyles.rule7]}>
                  {peer.error}
                </p>
              )}
              {showDetails && peer.peerId && (
                <CopyableField
                  sx={nativeChildSpacing.s0_5}
                  label="Peer ID"
                  value={peer.peerId}
                />
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// peerStatus says whether the stream runs and since when.
function peerStatus(peer: SOSyncPeer): string {
  const state = peer.connected ? 'Connected' : 'Disconnected'
  if (!peer.since) return state
  const since = peer.since.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
  return `${state} since ${since}`
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
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
