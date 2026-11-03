import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { useCallback, useState } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuKeyRound,
  LuRotateCw,
  LuSkipForward,
} from '@s4wave/web/images/icons.js'

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

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { color: themeColors['--color-brand'] },
  rule2: { color: themeColors['--color-destructive'] },
  rule3: { color: themeColors['--color-warning'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// SessionSelfEnrollmentStatusButton registers the self-enrollment bottom-bar item.
export function SessionSelfEnrollmentStatusButton() {
  const status = useSessionSelfEnrollmentStatus()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <Popover open={selected}>
        <PopoverAnchor asChild>
          <BottomBarItem
            selected={selected}
            onClick={onClick}
            sx={[
              sx,
              !!status.running && stylexStyles2.rule1,
              !!status.failed && stylexStyles2.rule2,
              !!status.credentialRequired && stylexStyles2.rule3,
            ]}
            aria-label={status.ariaLabel}
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
          variant="status"
        >
          <SessionSelfEnrollmentStatusPopover status={status} />
        </PopoverContent>
      </Popover>
    ),
    [status],
  )

  if (!status.visible) return null

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
    return <LuCircleAlert sx={stylexStyles.rule1} aria-hidden="true" />
  }
  if (status.credentialRequired) {
    return <LuKeyRound sx={stylexStyles.rule1} aria-hidden="true" />
  }
  if (status.skipped) {
    return <LuSkipForward sx={stylexStyles.rule1} aria-hidden="true" />
  }
  return <LuCircleCheck sx={stylexStyles.rule1} aria-hidden="true" />
}

// useSelfEnrollmentActions runs the start and skip actions of the status
// resource, tracking the busy flag and the last action error.
function useSelfEnrollmentActions(status: SessionSelfEnrollmentStatusView) {
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)

  const run = useCallback(async (action: () => Promise<unknown>) => {
    setBusy(true)
    setActionError('')
    try {
      await action()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [])

  const start = () => {
    const { resource } = status
    if (!resource || busy) return
    void run(() => resource.start())
  }
  const skip = () => {
    const { resource, generationKey } = status
    if (!resource || !generationKey || busy) return
    void run(() => resource.skip(generationKey))
  }

  return { actionError, busy, start, skip }
}

function SessionSelfEnrollmentStatusPopover({
  status,
}: {
  status: SessionSelfEnrollmentStatusView
}) {
  const { actionError, busy, start, skip } = useSelfEnrollmentActions(status)

  return (
    <div
      className="space-y-3 p-3"
      data-testid="session-self-enrollment-status-popover"
    >
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-md',
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

      {status.progressVisible && (
        <ProgressBar
          value={status.progress}
          indeterminate={status.progressIndeterminate}
        />
      )}

      <FailedSpacesList failures={status.failures} />

      {actionError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-2 py-1.5 text-xs">
          {actionError}
        </div>
      )}

      {status.startVisible && (
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!status.resource || busy || status.running}
            onClick={start}
          >
            {busy ? (
              <Spinner size="sm" />
            ) : (
              <LuRotateCw sx={stylexStyles.rule1} aria-hidden="true" />
            )}
            {status.startLabel}
          </Button>
          {status.skipVisible && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!status.resource || busy || status.running}
              onClick={skip}
            >
              <LuSkipForward sx={stylexStyles.rule1} aria-hidden="true" />
              Skip
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

// FailedSpacesList lists the first three spaces that failed to enroll.
function FailedSpacesList({
  failures,
}: {
  failures: SessionSelfEnrollmentStatusView['failures']
}) {
  if (failures.length === 0) return null

  return (
    <div className="border-foreground/8 space-y-1.5 border-t pt-2">
      <div className="text-foreground-alt/50 micro-text font-medium tracking-widest uppercase">
        Failed spaces
      </div>
      {failures.slice(0, 3).map((failure) => (
        <div
          key={failure.sharedObjectId}
          className="text-foreground-alt/70 flex items-start justify-between gap-3 text-xs"
        >
          <span className="min-w-0 truncate font-mono">
            {failure.sharedObjectId}
          </span>
          <span className="text-destructive text-right">{failure.message}</span>
        </div>
      ))}
    </div>
  )
}
