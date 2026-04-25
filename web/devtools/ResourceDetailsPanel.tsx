import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuCopy, LuRefreshCw, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  useResourceDevToolsContext,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

interface ResourceDetailsPanelProps {
  resource: TrackedResource
}

// ResourceDetailsPanel displays detailed information about a selected resource.
export function ResourceDetailsPanel({ resource }: ResourceDetailsPanelProps) {
  const devtools = useResourceDevToolsContext()

  const handleRetry = useCallback(() => {
    resource.retry()
  }, [resource])

  const handleCopy = useCallback(() => {
    const data = {
      id: resource.id,
      resourceId: resource.resourceId,
      resourceType: resource.resourceType,
      state: resource.state,
      released: resource.released,
      debugLabel: resource.debugLabel,
      debugDetails: resource.debugDetails,
      error:
        resource.error ?
          {
            message: resource.error.message,
            stack: resource.error.stack,
          }
        : null,
      createdAt: new Date(resource.createdAt).toISOString(),
      parentIds: resource.parentIds,
    }
    void navigator.clipboard.writeText(JSON.stringify(data, null, 2))
  }, [resource])

  const handleClose = useCallback(() => {
    devtools?.setSelectedId(null)
  }, [devtools])

  const createdAtStr = useMemo(() => {
    const date = new Date(resource.createdAt)
    return date.toLocaleTimeString()
  }, [resource.createdAt])

  // Live-updating duration for loading resources
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (resource.state !== 'loading') return
    const interval = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(interval)
  }, [resource.state])

  const durationStr = useMemo(() => {
    const elapsed = now - resource.createdAt
    const secs = Math.floor(elapsed / 1000)
    if (secs < 60) return `${secs}s`
    const mins = Math.floor(secs / 60)
    const remainingSecs = secs % 60
    return `${mins}m ${remainingSecs}s`
  }, [resource.createdAt, now])

  const stateColor = useMemo(() => {
    if (resource.state === 'ready') return 'text-devtools-success'
    if (resource.state === 'loading') return 'text-devtools-warning'
    if (resource.state === 'error') return 'text-devtools-error'
    return 'text-text-secondary'
  }, [resource.state])

  return (
    <div
      className={cn(
        'border-popover-border flex w-56 shrink-0 flex-col overflow-hidden border-l',
      )}
      data-testid="resource-details-panel"
    >
      {/* Header */}
      <div
        className={cn(
          'border-popover-border/50 flex h-6 shrink-0 items-center justify-between border-b px-2',
          'bg-background-deep/30',
        )}
      >
        <span className="text-text-secondary text-xs font-medium">Details</span>
        <button
          type="button"
          onClick={handleClose}
          className={cn(
            'flex h-4 w-4 items-center justify-center rounded',
            'text-foreground-alt hover:text-foreground hover:bg-pulldown-hover',
            'transition-colors duration-100',
          )}
        >
          <LuX className="h-3 w-3" />
        </button>
      </div>

      {/* Content */}
      <div className="flex flex-1 flex-col gap-3 overflow-auto p-3">
        {/* Type & ID */}
        <div className="flex flex-col gap-1.5">
          <DetailRow
            label="Type"
            value={resource.resourceType ?? '(unknown)'}
            mono
          />
          <DetailRow
            label="ID"
            value={
              resource.resourceId != null ? `#${resource.resourceId}` : '(none)'
            }
            mono
          />
        </div>

        {/* Status */}
        <div className="flex flex-col gap-1.5">
          <DetailRow
            label="State"
            value={resource.state}
            valueClassName={stateColor}
          />
          {resource.state === 'loading' && (
            <DetailRow
              label="Duration"
              value={durationStr}
              valueClassName="text-devtools-warning"
            />
          )}
          <DetailRow
            label="Released"
            value={resource.released ? 'Yes' : 'No'}
            valueClassName={
              resource.released ? 'text-devtools-muted' : undefined
            }
          />
          <DetailRow label="Created" value={createdAtStr} />
        </div>

        {/* Debug Details */}
        {resource.debugDetails &&
          Object.keys(resource.debugDetails).length > 0 && (
            <div className="flex flex-col gap-1.5">
              <span className="text-foreground-alt text-xs font-medium">
                Debug Info
              </span>
              {Object.entries(resource.debugDetails).map(([key, value]) => (
                <DetailRow
                  key={key}
                  label={key}
                  value={value != null ? String(value) : '(null)'}
                  mono
                />
              ))}
            </div>
          )}

        {/* Error section */}
        {resource.error && (
          <div className="flex flex-col gap-1.5">
            <span className="text-error text-xs font-medium">Error</span>
            <div
              className={cn(
                'bg-error-bg border-error-border rounded border p-2 font-mono text-xs',
                'break-words',
              )}
            >
              {resource.error.message}
            </div>
            {resource.error.stack && (
              <pre
                className={cn(
                  'bg-background-deep text-foreground-alt max-h-24 overflow-auto rounded p-2 font-mono text-xs',
                  'break-words whitespace-pre-wrap',
                )}
              >
                {resource.error.stack}
              </pre>
            )}
          </div>
        )}
      </div>

      {/* Actions footer */}
      <div
        className={cn(
          'border-popover-border/50 flex shrink-0 gap-1 border-t p-2',
          'bg-background-deep/30',
        )}
      >
        <ActionButton onClick={handleRetry} icon={<LuRefreshCw />}>
          Retry
        </ActionButton>
        <ActionButton onClick={handleCopy} icon={<LuCopy />}>
          Copy
        </ActionButton>
      </div>
    </div>
  )
}

interface DetailRowProps {
  label: string
  value: string
  mono?: boolean
  valueClassName?: string
}

function DetailRow({ label, value, mono, valueClassName }: DetailRowProps) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-foreground-alt text-xs">{label}</span>
      <span
        className={cn(
          'text-foreground truncate text-xs',
          mono && 'font-mono',
          valueClassName,
        )}
      >
        {value}
      </span>
    </div>
  )
}

interface ActionButtonProps {
  onClick: () => void
  icon: React.ReactNode
  children: React.ReactNode
}

function ActionButton({ onClick, icon, children }: ActionButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex flex-1 items-center justify-center gap-1 rounded py-1 text-xs',
        'bg-background-deep text-text-secondary',
        'hover:bg-pulldown-hover hover:text-foreground',
        'transition-colors duration-100',
      )}
    >
      <span className="[&>svg]:h-3 [&>svg]:w-3">{icon}</span>
      {children}
    </button>
  )
}
