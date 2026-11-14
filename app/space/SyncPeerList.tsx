import type { SOSyncPeer } from '@s4wave/core/sobject/sobject.pb.js'
import type { SpaceSharingState } from '@s4wave/sdk/space/space.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'

import { deviceName } from './device-name.js'

// SyncPeerList shows this device's sync stream with each other device that
// has synced the Space since this device started, and whether it still runs.
// Peer IDs show only with the technical details.
export function SyncPeerList({
  state,
  peers,
  showDetails,
}: {
  state: SpaceSharingState
  peers: SOSyncPeer[]
  showDetails: boolean
}) {
  return (
    <div className="space-y-1">
      <h4 className="text-foreground-alt text-xs select-none">Other devices</h4>
      {peers.length === 0 ? (
        <p className="text-foreground-alt text-xs">
          No other device has synced since this device started.
        </p>
      ) : (
        <ul className="space-y-1" aria-label="Other devices">
          {peers.map((peer) => (
            <li key={peer.peerId} className="space-y-0.5 text-xs">
              <div className="flex flex-wrap justify-between gap-x-2">
                <span className="text-foreground">
                  {deviceName(state, peer.peerId ?? '', 'Unknown device')}
                </span>
                <span
                  className={cn(
                    peer.connected ? 'text-foreground-alt' : 'text-warning',
                  )}
                >
                  {peerStatus(peer)}
                </span>
              </div>
              {!peer.connected && peer.error && (
                <p className="text-warning break-words">{peer.error}</p>
              )}
              {showDetails && peer.peerId && (
                <FactList>
                  <FactRow label="Peer ID" value={peer.peerId} />
                </FactList>
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
