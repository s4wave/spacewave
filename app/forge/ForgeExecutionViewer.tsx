import { useEffect, useMemo, useRef, useState } from 'react'
import { LuActivity } from 'react-icons/lu'

import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'

export const ForgeExecutionTypeID = 'forge/execution'

const execStateLabels: Record<number, string> = {
  [State.ExecutionState_UNKNOWN]: 'UNKNOWN',
  [State.ExecutionState_PENDING]: 'PENDING',
  [State.ExecutionState_RUNNING]: 'RUNNING',
  [State.ExecutionState_COMPLETE]: 'COMPLETE',
}

// logLevelColor returns a text color class for a log level.
function logLevelColor(level: string): string {
  switch (level) {
    case 'error':
      return 'text-destructive'
    case 'warn':
      return 'text-amber-300'
    case 'debug':
      return 'text-foreground-alt/50'
    default:
      return 'text-foreground'
  }
}

function describeResult(result?: {
  success?: boolean
  canceled?: boolean
  failError?: string
}): string {
  if (!result) return 'No result recorded yet'
  if (result.canceled ?? false) return 'Canceled'
  if (result.success ?? false) return 'Success'
  if (result.failError) return result.failError
  return 'Failed'
}

function formatDuration(timestamp: Date, now: number): string {
  const elapsedMs = Math.max(0, now - timestamp.getTime())
  const totalSeconds = Math.floor(elapsedMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

function ExecutionDuration({
  timestamp,
  running,
}: {
  timestamp?: Date
  running: boolean
}) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!timestamp || !running) return

    const interval = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(interval)
    }
  }, [running, timestamp])

  if (!timestamp) {
    return <span className="text-foreground-alt/50 text-xs">Not started</span>
  }

  return (
    <span className="text-foreground text-sm font-medium">
      {formatDuration(timestamp, now)}
    </span>
  )
}

// LogViewer renders execution log entries with auto-scroll.
function LogViewer({ logEntries }: { logEntries: Execution['logEntries'] }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const entries = logEntries ?? []

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [entries.length])

  if (entries.length === 0) {
    return (
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
          <LuActivity className="h-3.5 w-3.5 shrink-0" />
          <span>No log output</span>
        </div>
      </div>
    )
  }

  return (
    <div
      ref={scrollRef}
      className="border-foreground/6 bg-background/40 max-h-64 overflow-y-auto rounded-lg border p-3"
    >
      {entries.map((entry, i) => (
        <div key={i} className="flex gap-2 font-mono text-xs leading-relaxed">
          {entry.level && (
            <span className={logLevelColor(entry.level)}>
              [{entry.level.toUpperCase()}]
            </span>
          )}
          <span className="text-foreground flex-1 break-all whitespace-pre-wrap">
            {entry.message}
          </span>
        </div>
      ))}
    </div>
  )
}

// ForgeExecutionViewer displays a Forge Execution entity.
export function ForgeExecutionViewer({
  objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const execution = useForgeBlockData(objectState, Execution)
  const isRunning =
    (execution?.executionState ?? State.ExecutionState_UNKNOWN) ===
    State.ExecutionState_RUNNING

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'logs',
        label: 'Logs',
        content: <LogViewer logEntries={execution?.logEntries} />,
      },
      {
        id: 'inputs',
        label: 'Inputs',
        content: (
          <div className="space-y-3">
            <ForgeValueSetDisplay
              title="Inputs"
              values={execution?.valueSet?.inputs}
              emptyLabel="No inputs recorded"
            />
          </div>
        ),
      },
      {
        id: 'outputs',
        label: 'Outputs',
        content: (
          <div className="space-y-3">
            <ForgeValueSetDisplay
              title="Outputs"
              values={execution?.valueSet?.outputs}
              emptyLabel="No outputs recorded"
            />
          </div>
        ),
      },
      {
        id: 'details',
        label: 'Details',
        content: (
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                {execution?.peerId && (
                  <CopyableField
                    label="Worker Peer ID"
                    value={execution.peerId}
                  />
                )}
                {execution?.timestamp && (
                  <CopyableField
                    label="Created"
                    value={execution.timestamp.toISOString()}
                  />
                )}
                <CopyableField label="Object Key" value={objectKey} />
              </div>
            </InfoCard>
            <InfoCard title="Runtime">
              <div className="flex items-center justify-between gap-3">
                <div className="text-foreground-alt/50 text-xs">
                  Live duration
                </div>
                <ExecutionDuration
                  timestamp={execution?.timestamp}
                  running={isRunning}
                />
              </div>
              <div className="text-foreground-alt/50 mt-3 flex flex-wrap gap-3 text-xs">
                <span>{execution?.logEntries?.length ?? 0} log entries</span>
                <span>{execution?.valueSet?.inputs?.length ?? 0} inputs</span>
                <span>{execution?.valueSet?.outputs?.length ?? 0} outputs</span>
              </div>
            </InfoCard>
            <InfoCard title="Result">
              <div className="text-foreground text-sm font-medium">
                {describeResult(execution?.result)}
              </div>
            </InfoCard>
          </div>
        ),
      },
    ],
    [execution, isRunning, objectKey],
  )

  return (
    <ForgeViewerShell
      icon={<LuActivity className="h-4 w-4" />}
      title="Execution"
      state={execution?.executionState ?? 0}
      stateLabels={execStateLabels}
      tabs={tabs}
    />
  )
}
