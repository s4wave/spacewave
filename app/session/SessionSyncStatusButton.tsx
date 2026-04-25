import { useCallback } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from './SessionSyncStatusContext.js'

// SessionSyncStatusButton registers the session sync-status bottom-bar item.
export function SessionSyncStatusButton() {
  const status = useSessionSyncStatus()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
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
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          sideOffset={6}
          onEscapeKeyDown={onClick}
          onPointerDownOutside={onClick}
          className="border-foreground/15 bg-background-card text-foreground z-50 w-80 max-w-[calc(100vw-1rem)] rounded-lg p-0 shadow-xl backdrop-blur-md"
        >
          <SessionSyncStatusPopover status={status} />
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

function SessionSyncStatusGlyph({ status }: { status: SessionSyncStatusView }) {
  if (status.loading || status.active) {
    return <Spinner size="sm" />
  }
  if (status.error) {
    return <LuCircleAlert className="h-3.5 w-3.5" aria-hidden="true" />
  }
  const Icon = status.local ? LuHardDrive : LuCloud
  return (
    <span
      className="relative flex h-3.5 w-3.5 items-center justify-center"
      aria-hidden="true"
    >
      <Icon className="h-3 w-3" />
      <LuCircleCheck className="text-brand absolute -right-1 -bottom-1 h-2 w-2" />
    </span>
  )
}

function SessionSyncStatusPopover({
  status,
}: {
  status: SessionSyncStatusView
}) {
  return (
    <div className="space-y-3 p-3" data-testid="session-sync-status-popover">
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
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
        <SyncMetric label="Uploading now" value={status.activeUploadLabel} />
        <SyncMetric label="Upload queued" value={status.pendingUploadLabel} />
        <SyncMetric
          label="Download queued"
          value={status.pendingDownloadLabel}
        />
      </div>

      <div className="border-foreground/8 space-y-1.5 border-t pt-2">
        <div className="text-foreground-alt/50 text-[0.6rem] font-medium tracking-widest uppercase">
          Pack reads
        </div>
        <SyncRow label="Ranges" value={status.packRangeLabel} />
        <SyncRow label="Index tail" value={status.packIndexTailLabel} />
        <SyncRow label="Lookup" value={status.packLookupLabel} />
        <SyncRow label="Index cache" value={status.packIndexCacheLabel} />
      </div>

      <div className="border-foreground/8 space-y-1.5 border-t pt-2">
        <SyncRow label="Transport" value={status.transportLabel} />
        <SyncRow label="P2P" value={status.p2pLabel} />
        <SyncRow label="Activity" value={status.lastActivityLabel} />
        {status.lastError && (
          <SyncRow label="Last error" value={status.lastError} tone="error" />
        )}
      </div>
    </div>
  )
}

function SyncMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/6 bg-foreground/5 rounded-md border px-2 py-1.5">
      <div className="text-foreground-alt/50 text-[0.6rem] font-medium tracking-widest uppercase">
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
