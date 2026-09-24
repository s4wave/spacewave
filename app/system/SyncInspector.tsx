import type { SessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'

// SyncInspector shows every reading of the session sync watcher: status,
// throughput, local copies, peers, and pack engine counters.
export function SyncInspector({ sync }: { sync: SessionSyncStatusView }) {
  return (
    <div className="grid gap-3 @3xl:grid-cols-2">
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
          <p className="text-foreground-alt/50 text-xs">
            No local copies reported.
          </p>
        ) : (
          <ul className="divide-foreground/6 divide-y">
            {sync.localCopies.map((copy) => (
              <li key={copy.id} className="py-2 first:pt-0 last:pb-0">
                <div className="flex items-baseline justify-between gap-3 text-xs">
                  <span className="text-foreground truncate font-medium">
                    {copy.name}
                  </span>
                  <span className="text-foreground-alt/70 shrink-0">
                    {copy.status}
                  </span>
                </div>
                <p className="text-foreground-alt/60 mt-0.5 font-mono text-xs tabular-nums">
                  {copy.detail}
                </p>
                {copy.error && (
                  <p className="text-destructive mt-0.5 text-xs">
                    {copy.error}
                  </p>
                )}
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
          <p className="text-foreground-alt/50 text-xs">
            No peers are exchanging data right now.
          </p>
        ) : (
          <ul className="divide-foreground/6 divide-y">
            {sync.peers.map((peer) => (
              <li key={peer.id} className="py-2 first:pt-0 last:pb-0">
                <div className="flex items-baseline justify-between gap-3 text-xs">
                  <span className="text-foreground truncate font-medium">
                    {peer.name}
                  </span>
                  <span className="text-foreground-alt/70 shrink-0">
                    {peer.state}
                  </span>
                </div>
                <p className="text-foreground-alt/60 mt-0.5 font-mono text-xs tabular-nums">
                  {peer.traffic}
                </p>
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
