import { useCallback, useState } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuKeyRound,
  LuRotateCw,
  LuSkipForward,
} from 'react-icons/lu'

import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Button } from '@s4wave/web/ui/button.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  type SessionSelfEnrollmentStatusView,
  useSessionSelfEnrollmentStatus,
} from './SessionSelfEnrollmentStatusContext.js'

// SessionSelfEnrollmentStatusButton registers the self-enrollment bottom-bar item.
export function SessionSelfEnrollmentStatusButton() {
  const status = useSessionSelfEnrollmentStatus()
  const visible =
    status.loading || status.visualState !== 'ready' || status.totalCount > 0
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            className={cn(
              className,
              status.running && 'text-brand',
              status.failed && 'text-destructive',
              status.credentialRequired && 'text-warning',
            )}
            aria-label={`Session self-enrollment status: ${status.summaryLabel}`}
            data-testid="session-self-enrollment-status-button"
          >
            <SessionSelfEnrollmentStatusGlyph status={status} />
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
          <SessionSelfEnrollmentStatusPopover status={status} />
        </PopoverContent>
      </Popover>
    ),
    [status],
  )

  if (!visible) return null

  return (
    <BottomBarLevel
      id="session-self-enrollment-status"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

function SessionSelfEnrollmentStatusGlyph({
  status,
}: {
  status: SessionSelfEnrollmentStatusView
}) {
  if (status.loading || status.running) {
    return <Spinner size="sm" />
  }
  if (status.failed) {
    return <LuCircleAlert className="h-3.5 w-3.5" aria-hidden="true" />
  }
  if (status.credentialRequired) {
    return <LuKeyRound className="h-3.5 w-3.5" aria-hidden="true" />
  }
  if (status.skipped) {
    return <LuSkipForward className="h-3.5 w-3.5" aria-hidden="true" />
  }
  return <LuCircleCheck className="h-3.5 w-3.5" aria-hidden="true" />
}

function SessionSelfEnrollmentStatusPopover({
  status,
}: {
  status: SessionSelfEnrollmentStatusView
}) {
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)
  const handleStart = useCallback(async () => {
    if (!status.resource || busy) return
    setBusy(true)
    setActionError('')
    try {
      await status.resource.start()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [busy, status.resource])
  const handleSkip = useCallback(async () => {
    if (!status.resource || !status.generationKey || busy) return
    setBusy(true)
    setActionError('')
    try {
      await status.resource.skip(status.generationKey)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [busy, status.generationKey, status.resource])

  return (
    <div
      className="space-y-3 p-3"
      data-testid="session-self-enrollment-status-popover"
    >
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
            status.failed && 'bg-destructive/10 text-destructive',
            status.running && 'bg-brand/10 text-brand',
            status.credentialRequired && 'bg-warning/10 text-warning',
            status.skipped && 'bg-foreground/5 text-foreground-alt',
            status.visualState === 'pending' && 'bg-brand/10 text-brand',
          )}
        >
          <SessionSelfEnrollmentStatusGlyph status={status} />
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

      {(status.running ||
        status.pending ||
        status.skipped ||
        status.failed) && (
        <ProgressBar
          value={status.progress}
          indeterminate={status.running && status.totalCount === 0}
        />
      )}

      {status.failures.length > 0 && (
        <div className="border-foreground/8 space-y-1.5 border-t pt-2">
          <div className="text-foreground-alt/50 text-[0.6rem] font-medium tracking-widest uppercase">
            Failed spaces
          </div>
          {status.failures.slice(0, 3).map((failure) => (
            <div
              key={failure.sharedObjectId}
              className="text-foreground-alt/70 flex items-start justify-between gap-3 text-xs"
            >
              <span className="min-w-0 truncate font-mono">
                {failure.sharedObjectId}
              </span>
              <span className="text-destructive text-right">
                {failure.message}
              </span>
            </div>
          ))}
        </div>
      )}

      {actionError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-2 py-1.5 text-xs">
          {actionError}
        </div>
      )}

      {(status.pending || status.failed || status.skipped) && (
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!status.resource || busy || status.running}
            onClick={() => {
              void handleStart()
            }}
          >
            {busy ?
              <Spinner size="sm" />
            : <LuRotateCw className="h-3.5 w-3.5" aria-hidden="true" />}
            {status.failed || status.skipped ? 'Retry' : 'Connect'}
          </Button>
          {status.generationKey && !status.skipped && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!status.resource || busy || status.running}
              onClick={() => {
                void handleSkip()
              }}
            >
              <LuSkipForward className="h-3.5 w-3.5" aria-hidden="true" />
              Skip
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
