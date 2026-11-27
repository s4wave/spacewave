import { useCallback } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useBottomBarDismissMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  type SessionSyncSummary,
  useSessionSyncStatus,
  useSessionSyncSummary,
} from './SessionSyncStatusContext.js'

const menuId = 'session-sync-status'

// SessionSyncStatusButton registers the session sync-status bottom-bar item.
// The button follows the sync summary. Only the open popover follows the
// transfer rates.
export function SessionSyncStatusButton() {
  const dismissMenu = useBottomBarDismissMenu(menuId)
  const status = useSessionSyncSummary()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <Popover
        open={selected}
        onOpenChange={(open) => (open ? onClick() : dismissMenu())}
      >
        <PopoverTrigger asChild>
          <BottomBarItem
            selected={selected}
            className={cn(
              className,
              status.active && 'text-brand',
              status.error && 'text-destructive',
            )}
            aria-label={status.ariaLabel}
            data-testid="session-sync-status-button"
          >
            <SessionSyncStatusGlyph status={status} />
          </BottomBarItem>
        </PopoverTrigger>
        <PopoverContent side="top" align="end" sideOffset={6} variant="status">
          <SessionSyncStatusPopover />
        </PopoverContent>
      </Popover>
    ),
    [status, dismissMenu],
  )

  return (
    <BottomBarLevel id={menuId} position="right" button={buttonRender}>
      {null}
    </BottomBarLevel>
  )
}

function SessionSyncStatusGlyph({ status }: { status: SessionSyncSummary }) {
  if (status.loading || status.active) {
    return <Spinner size="sm" />
  }
  if (status.error) {
    return <LuCircleAlert className="size-3.5" aria-hidden="true" />
  }
  const Icon = status.local ? LuHardDrive : LuCloud
  return (
    <span
      className="relative flex size-3.5 items-center justify-center"
      aria-hidden="true"
    >
      <Icon className="size-3" />
      <LuCircleCheck className="text-brand absolute -right-1 -bottom-1 size-2" />
    </span>
  )
}

function SessionSyncStatusPopover() {
  const status = useSessionSyncStatus()
  return (
    <div className="space-y-3 p-3" data-testid="session-sync-status-popover">
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-md',
            status.error && 'bg-destructive/10 text-destructive',
            status.active && 'bg-brand/10 text-brand',
            status.visualState === 'synced' && 'bg-foreground/5 text-brand',
            status.loading && 'bg-foreground/5 text-foreground-alt',
          )}
        >
          <SessionSyncStatusGlyph status={status} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold tracking-tight">
            {status.summaryLabel}
          </div>
          <div className="text-foreground-alt/60 mt-0.5 text-xs">
            {status.detailLabel}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2">
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
        <div className="border-foreground/8 max-h-56 space-y-3 overflow-y-auto border-t pt-2">
          {status.localCopies.map((copy) => (
            <div key={copy.id}>
              <SyncRow label={copy.name} value={copy.status} />
              <div className="text-foreground-alt/50 mt-0.5 text-xs">
                {copy.detail}
              </div>
              {copy.error && (
                <details className="text-foreground-alt/60 mt-1 text-xs">
                  <summary className="cursor-pointer [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
                    Copy details
                  </summary>
                  <p className="mt-1 break-words">{copy.error}</p>
                </details>
              )}
            </div>
          ))}
        </div>
      )}

      {!status.local && (
        <details className="border-foreground/8 space-y-1.5 border-t pt-2">
          <summary className="text-foreground-alt/50 cursor-pointer text-xs [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
            Storage diagnostics
          </summary>
          <SyncRow label="Ranges" value={status.packRangeLabel} />
          <SyncRow label="Index tail" value={status.packIndexTailLabel} />
          <SyncRow label="Lookup" value={status.packLookupLabel} />
          <SyncRow label="Index cache" value={status.packIndexCacheLabel} />
        </details>
      )}

      {status.peers.length > 0 && (
        <details className="border-foreground/8 space-y-2 border-t pt-2">
          <summary className="text-foreground-alt/50 cursor-pointer text-xs [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
            Linked Sessions
          </summary>
          {status.peers.map((peer) => (
            <div key={peer.id}>
              <SyncRow label={peer.name} value={peer.state} />
              <div className="text-foreground-alt/50 mt-0.5 text-right text-xs">
                {peer.traffic}
              </div>
            </div>
          ))}
        </details>
      )}

      <div className="border-foreground/8 space-y-1.5 border-t pt-2">
        <SyncRow
          label={status.local ? 'Network' : 'Account access'}
          value={status.transportLabel}
        />
        <SyncRow label="P2P" value={status.p2pLabel} />
        {status.peerError && !status.local && (
          <SyncRow label="Peer error" value={status.peerError} tone="error" />
        )}
        <SyncRow label="Activity" value={status.lastActivityLabel} />
        {status.lastError && (
          <SyncRow
            label={status.local ? 'Peer error' : 'Cloud error'}
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
    <div className="border-foreground/6 bg-foreground/5 rounded-md border px-2 py-1.5">
      <div className="text-foreground-alt/50 micro-text font-medium tracking-widest uppercase">
        {label}
      </div>
      <div className="text-xs font-semibold">{value}</div>
    </div>
  )
}

function SyncRow({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'error'
}) {
  return (
    <div className="flex items-start justify-between gap-3 text-xs">
      <span className="text-foreground-alt/50 shrink-0">{label}</span>
      <span
        className={cn(
          'text-right break-words',
          tone === 'error' ? 'text-destructive' : 'text-foreground-alt/80',
        )}
      >
        {value}
      </span>
    </div>
  )
}
