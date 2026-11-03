import type { WatchNetworkStatsResponse } from '@s4wave/sdk/status/status.pb.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { plural } from './format.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { gap: `calc(${themeLayout['--spacing']} * 3)`, display: 'grid' },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-success'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule5: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textAlign: 'left',
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    paddingBlock: themeLayout['--spacing'],
    fontWeight: 'var(--font-weight-normal)',
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    paddingBlock: themeLayout['--spacing'],
    fontWeight: 'var(--font-weight-normal)',
  },
  rule11: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 85%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule13: {
    paddingBlock: themeLayout['--spacing'],
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule14: { paddingBlock: themeLayout['--spacing'] },
})

// NetworkInspector shows the session transport, this device's peer identity,
// and every live link to every remote peer.
export function NetworkInspector({
  network,
}: {
  network: WatchNetworkStatsResponse | null
}) {
  const peers = network?.peers ?? []

  return (
    <div sx={stylexStyles.rule1}>
      <InspectorSection
        title="Transport"
        description="The peer transport carries encrypted links between your devices."
      >
        <Facts
          empty="Waiting for the network watcher."
          facts={
            network
              ? [
                  {
                    label: 'Status',
                    value: network.transportRunning ? 'Running' : 'Stopped',
                    tone: network.transportRunning ? undefined : 'warning',
                  },
                  {
                    label: 'This device',
                    value: network.localPeerId || 'Not assigned',
                    mono: !!network.localPeerId,
                  },
                  {
                    label: 'Peers',
                    value: network.peerCount ?? 0,
                    mono: true,
                  },
                  {
                    label: 'Links',
                    value: network.linkCount ?? 0,
                    mono: true,
                  },
                ]
              : []
          }
        />
      </InspectorSection>

      <InspectorSection
        title="Peers and links"
        description="Each link is one live connection between a local and a remote transport."
      >
        {peers.length === 0 ? (
          <p sx={stylexStyles.rule2}>No peers are connected.</p>
        ) : (
          <ul>
            {peers.map((peer) => {
              const links = peer.links ?? []
              return (
                <li sx={nativeChildSpacing.s3} key={peer.peerId}>
                  <div sx={stylexStyles.rule3}>
                    <span sx={stylexStyles.rule4} aria-hidden="true" />
                    <span sx={stylexStyles.rule5}>{peer.peerId}</span>
                    <span sx={stylexStyles.rule6}>
                      {plural(peer.linkCount ?? links.length, 'link')}
                    </span>
                  </div>
                  <table sx={stylexStyles.rule7}>
                    <thead sx={stylexStyles.rule8}>
                      <tr>
                        <th sx={stylexStyles.rule9}>Link</th>
                        <th sx={stylexStyles.rule9}>Transport</th>
                        <th sx={stylexStyles.rule10}>Remote transport</th>
                      </tr>
                    </thead>
                    <tbody sx={stylexStyles.rule11}>
                      {links.map((link) => (
                        <tr key={String(link.linkId)} sx={stylexStyles.rule12}>
                          <td sx={stylexStyles.rule13}>
                            {String(link.linkId)}
                          </td>
                          <td sx={stylexStyles.rule13}>
                            {String(link.transportId)}
                          </td>
                          <td sx={stylexStyles.rule14}>
                            {String(link.remoteTransportId)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </li>
              )
            })}
          </ul>
        )}
      </InspectorSection>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
