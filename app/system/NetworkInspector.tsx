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
  rule1: { display: 'grid', gap: `calc(${themeLayout['--spacing']} * 3)` },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    flexShrink: 0,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-success'],
  },
  rule5: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    textAlign: 'left',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    paddingBlock: themeLayout['--spacing'],
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
  },
  rule10: {
    paddingBlock: themeLayout['--spacing'],
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
  },
  rule11: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 85%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule12: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
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
          <ul
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
            }
          >
            {peers.map((peer) => {
              const links = peer.links ?? []
              return (
                <li key={peer.peerId}>
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
