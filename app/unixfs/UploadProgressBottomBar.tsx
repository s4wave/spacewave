import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LuCheck, LuCircleAlert, LuFolder, LuUpload, LuX } from 'react-icons/lu'

import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import type {
  UploadEvent,
  UploadManager,
  UploadItem,
} from './useUploadManager.js'

// Auto-dismiss windows for the anchored feedback popover. Completion lingers
// slightly longer so the "added" confirmation reads before it fades; both are
// capped by the indicator itself unmounting once the manager clears its items.
const STARTED_DISMISS_MS = 3500
const COMPLETED_DISMISS_MS = 5000

// formatBytes formats a byte count into a human-readable string.
function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

function formatUploadError(error: string | undefined): string | undefined {
  if (
    !error?.includes('QuotaExceededError') &&
    !error?.includes('browser storage quota exceeded')
  ) {
    return error
  }
  const available = error.match(/available (\d+) bytes/)
  const required = error.match(/write (\d+) bytes/)
  const headroom = available
    ? ` (${formatBytes(Number(available[1]))} available${
        required ? `; ${formatBytes(Number(required[1]))} needed` : ''
      })`
    : ''
  return `Browser storage quota was exceeded${headroom}. Free storage used by this site or device, then retry. Spacewave already requested persistent storage; persistence prevents eviction but does not increase capacity.`
}

// UploadProgressOverlay renders the list of upload items with progress.
function UploadProgressOverlay({
  uploadManager,
}: {
  uploadManager: UploadManager
}) {
  const queuedCount = uploadManager.items.filter(
    (i) => i.status === 'queued',
  ).length
  const activeCount = uploadManager.items.filter(
    (i) => i.status === 'uploading',
  ).length
  const doneCount = uploadManager.items.filter(
    (i) => i.status === 'done',
  ).length
  const errorCount = uploadManager.items.filter(
    (i) => i.status === 'error',
  ).length
  const totalBytes = uploadManager.items.reduce(
    (sum, item) => sum + item.totalSize,
    0,
  )
  const writtenBytes = uploadManager.items.reduce(
    (sum, item) =>
      sum + (item.status === 'done' ? item.totalSize : item.bytesWritten),
    0,
  )
  const overallProgress =
    totalBytes > 0 ? Math.round((writtenBytes / totalBytes) * 100) : 0

  return (
    <div
      data-testid="upload-progress-overlay"
      className="bg-popover flex h-full min-h-0 w-full flex-col"
    >
      <div className="border-popover-border flex flex-wrap items-start justify-between gap-4 border-b px-5 py-4">
        <div className="min-w-0">
          <h2 className="text-foreground text-base font-semibold">Uploads</h2>
          <p className="text-foreground-alt mt-1 text-sm">
            Track file and folder uploads for this UnixFS view.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {uploadManager.items.some((i) => i.status === 'done') && (
            <button
              className="border-popover-border text-foreground-alt hover:text-foreground rounded-md border px-3 py-1.5 text-sm"
              onClick={uploadManager.clearDone}
            >
              Clear done
            </button>
          )}
          {uploadManager.items.some(
            (i) => i.status === 'queued' || i.status === 'uploading',
          ) && (
            <button
              className="border-destructive/30 text-destructive hover:text-destructive/80 rounded-md border px-3 py-1.5 text-sm"
              onClick={uploadManager.cancelAll}
            >
              Cancel all
            </button>
          )}
        </div>
      </div>

      <div className="border-popover-border grid grid-cols-2 gap-px border-b bg-[color:var(--color-popover-border)] sm:grid-cols-4">
        <SummaryCell label="Overall" value={`${overallProgress}%`} />
        <SummaryCell
          label="Active"
          value={
            activeCount > 0
              ? `${activeCount} uploading`
              : queuedCount > 0
                ? `${queuedCount} queued`
                : 'Idle'
          }
        />
        <SummaryCell label="Completed" value={`${doneCount}`} />
        <SummaryCell label="Failed" value={`${errorCount}`} />
      </div>

      <div className="border-popover-border px-5 py-4">
        <div className="bg-muted h-2 overflow-hidden rounded-full">
          <div
            className="bg-brand h-full rounded-full transition-[width] duration-200"
            style={{ width: `${overallProgress}%` }}
          />
        </div>
        <div className="text-foreground-alt mt-2 flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>
            {formatBytes(writtenBytes)} of {formatBytes(totalBytes)}
          </span>
          <span>{uploadManager.items.length} items</span>
        </div>
      </div>

      <div
        data-testid="upload-progress-list"
        className="min-h-0 flex-1 overflow-y-auto"
      >
        {uploadManager.items.map((item) => (
          <UploadItemRow
            key={item.id}
            item={item}
            onCancel={uploadManager.cancelUpload}
          />
        ))}
      </div>
    </div>
  )
}

function SummaryCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-popover min-w-0 px-5 py-4">
      <div className="text-foreground-alt text-xs tracking-[0.12em] uppercase">
        {label}
      </div>
      <div className="text-foreground mt-1 truncate text-lg font-semibold">
        {value}
      </div>
    </div>
  )
}

// UploadItemRow renders a single upload item row.
function UploadItemRow({
  item,
  onCancel,
}: {
  item: UploadItem
  onCancel: (id: string) => void
}) {
  const progress =
    item.totalSize > 0
      ? Math.round((item.bytesWritten / item.totalSize) * 100)
      : 0

  const handleCancel = useCallback(() => {
    onCancel(item.id)
  }, [item.id, onCancel])

  const statusLabel =
    item.status === 'uploading'
      ? 'Uploading'
      : item.status === 'queued'
        ? 'Queued'
        : item.status === 'done'
          ? 'Complete'
          : (formatUploadError(item.error) ?? 'Failed')

  return (
    <div className="border-popover-border flex items-start gap-3 border-t px-5 py-4 first:border-t-0">
      <div className="bg-muted text-foreground-alt mt-0.5 flex size-9 flex-shrink-0 items-center justify-center rounded-md">
        {item.kind === 'directory' ? (
          <LuFolder className="size-4" />
        ) : (
          <LuUpload className="size-4" />
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          <span className="text-foreground min-w-0 truncate text-sm font-medium">
            {item.name}
          </span>
          <span className="text-foreground-alt text-xs tracking-[0.12em] uppercase">
            {item.kind}
          </span>
          <span
            className={cn(
              'text-xs',
              item.status === 'done' && 'text-green-500',
              item.status === 'error' && 'text-destructive',
              (item.status === 'queued' || item.status === 'uploading') &&
                'text-foreground-alt',
            )}
          >
            {statusLabel}
          </span>
        </div>
        <div className="text-foreground-alt truncate text-xs">{item.path}</div>
        <div className="flex flex-wrap items-center gap-3">
          <div className="bg-muted h-1.5 min-w-[10rem] flex-1 overflow-hidden rounded-full">
            <div
              className="bg-brand h-full rounded-full transition-[width] duration-200"
              style={{ width: `${progress}%` }}
            />
          </div>
          <span className="text-foreground-alt text-xs tabular-nums">
            {formatBytes(
              item.status === 'done' ? item.totalSize : item.bytesWritten,
            )}
            {' / '}
            {formatBytes(item.totalSize)}
          </span>
          <span className="text-foreground-alt text-xs tabular-nums">
            {progress}%
          </span>
        </div>
      </div>
      <div className="flex size-8 flex-shrink-0 items-center justify-center">
        {(item.status === 'uploading' || item.status === 'queued') && (
          <button
            className="text-foreground-alt hover:text-foreground"
            onClick={handleCancel}
            title="Cancel"
          >
            <LuX className="size-4" />
          </button>
        )}
        {item.status === 'done' && (
          <LuCheck className="size-4 text-green-500" />
        )}
        {item.status === 'error' && (
          <button
            className="text-foreground-alt hover:text-foreground"
            onClick={handleCancel}
            title="Dismiss"
          >
            <LuX className="text-destructive size-4" />
          </button>
        )}
      </div>
    </div>
  )
}

// UploadFeedbackContent renders the anchored start/completion notification copy.
function UploadFeedbackContent({ feedback }: { feedback: UploadEvent }) {
  const started = feedback.kind === 'started'
  const hasErrors = feedback.errorCount > 0
  const title = started
    ? 'Upload started'
    : hasErrors
      ? 'Upload finished with errors'
      : 'Upload complete'
  const detail = started
    ? `Uploading ${feedback.fileCount} ${
        feedback.fileCount === 1 ? 'file' : 'files'
      }. Track progress here.`
    : hasErrors
      ? `${feedback.fileCount} uploaded, ${feedback.errorCount} failed.`
      : `${feedback.fileCount} ${
          feedback.fileCount === 1 ? 'file' : 'files'
        } added to this folder.`

  return (
    <div className="flex items-start gap-3" data-testid="upload-feedback">
      <div
        className={cn(
          'mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg',
          started && 'bg-brand/10 text-brand',
          !started && !hasErrors && 'bg-green-500/10 text-green-500',
          hasErrors && 'bg-destructive/10 text-destructive',
        )}
      >
        {started ? (
          <LuUpload className="size-4" />
        ) : hasErrors ? (
          <LuCircleAlert className="size-4" />
        ) : (
          <LuCheck className="size-4" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-foreground text-sm font-semibold">{title}</div>
        <div className="text-foreground-alt/70 mt-0.5 text-xs leading-relaxed">
          {detail}
        </div>
      </div>
    </div>
  )
}

// UploadProgressBottomBar renders an upload progress indicator in the bottom bar.
export function UploadProgressBottomBar({
  uploadManager,
}: {
  uploadManager: UploadManager
}) {
  const totalCount = uploadManager.items.length
  const activeUploading = uploadManager.activeCount
  const doneCount = useMemo(
    () => uploadManager.items.filter((i) => i.status === 'done').length,
    [uploadManager.items],
  )

  // Surface each upload-lifecycle event as a temporary popover anchored to the
  // indicator, then auto-dismiss. Deduping on event id keeps a re-render from
  // reopening a dismissed notification.
  const [feedback, setFeedback] = useState<UploadEvent | null>(null)
  const seenEventIdRef = useRef(0)
  const lastEvent = uploadManager.lastEvent
  useEffect(() => {
    if (!lastEvent || lastEvent.id === seenEventIdRef.current) return
    seenEventIdRef.current = lastEvent.id
    setFeedback(lastEvent)
    const dismissMs =
      lastEvent.kind === 'started' ? STARTED_DISMISS_MS : COMPLETED_DISMISS_MS
    const timer = setTimeout(() => setFeedback(null), dismissMs)
    return () => clearTimeout(timer)
  }, [lastEvent])

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <Popover open={feedback !== null}>
        <PopoverAnchor asChild>
          <button
            onClick={onClick}
            className={cn(
              'flex h-full shrink-0 items-center gap-1.5 px-2 text-xs whitespace-nowrap',
              selected && 'text-foreground',
              className,
            )}
          >
            {activeUploading > 0 ? (
              <Spinner size="sm" />
            ) : (
              <LuUpload className="size-3" />
            )}
            {activeUploading > 0
              ? `Uploading ${activeUploading}/${totalCount}`
              : `${doneCount}/${totalCount} uploaded`}
          </button>
        </PopoverAnchor>
        {feedback !== null && (
          <PopoverContent
            side="top"
            align="end"
            sideOffset={6}
            onOpenAutoFocus={(e) => e.preventDefault()}
            onCloseAutoFocus={(e) => e.preventDefault()}
            data-testid="upload-feedback-popover"
            className="w-72 p-3"
          >
            <UploadFeedbackContent feedback={feedback} />
          </PopoverContent>
        )}
      </Popover>
    ),
    [activeUploading, totalCount, doneCount, feedback],
  )

  if (totalCount === 0) return null

  return (
    <BottomBarLevel
      id="upload-progress"
      position="right"
      button={buttonRender}
      buttonKey={`${activeUploading}-${totalCount}-${doneCount}-${
        feedback ? feedback.id : 'none'
      }`}
      overlay={<UploadProgressOverlay uploadManager={uploadManager} />}
      overlayKey={`${totalCount}-${activeUploading}-${doneCount}`}
    >
      {null}
    </BottomBarLevel>
  )
}
