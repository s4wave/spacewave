import type { WatchNetworkStatsResponse } from '@s4wave/sdk/status/status.pb.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { plural } from './format.js'

// NetworkInspector shows the session transport, this device's peer identity,
// and every live link to every remote peer.
export function NetworkInspector({
  network,
}: {
  network: WatchNetworkStatsResponse | null
}) {
  const peers = network?.peers ?? []

  return (
    <div className="grid gap-3">
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
          <p className="text-foreground-alt/50 text-xs">
            No peers are connected.
          </p>
        ) : (
          <ul className="space-y-3">
            {peers.map((peer) => {
              const links = peer.links ?? []
              return (
                <li key={peer.peerId}>
                  <div className="flex items-center gap-2 text-xs">
                    <span
                      className="bg-success size-1.5 shrink-0 rounded-full"
                      aria-hidden="true"
                    />
                    <span className="text-foreground min-w-0 flex-1 truncate font-mono">
                      {peer.peerId}
                    </span>
                    <span className="text-foreground-alt/60 shrink-0">
                      {plural(peer.linkCount ?? links.length, 'link')}
                    </span>
                  </div>
                  <table className="mt-1.5 w-full text-xs">
                    <thead className="text-foreground-alt/50 text-left">
                      <tr>
                        <th className="py-1 pr-3 font-normal">Link</th>
                        <th className="py-1 pr-3 font-normal">Transport</th>
                        <th className="py-1 font-normal">Remote transport</th>
                      </tr>
                    </thead>
                    <tbody className="text-foreground/85 font-mono tabular-nums">
                      {links.map((link, index) => (
                        <tr
                          key={String(link.linkId ?? index)}
                          className="border-foreground/6 border-t"
                        >
                          <td className="py-1 pr-3">{String(link.linkId)}</td>
                          <td className="py-1 pr-3">
                            {String(link.transportId)}
                          </td>
                          <td className="py-1">
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
