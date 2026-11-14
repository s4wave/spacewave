import { useEffect, useMemo, useRef } from 'react'
import { LuActivity } from 'react-icons/lu'

import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlock } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  forgeBlockStatus,
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
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

function ExecutionTimestamp({ timestamp }: { timestamp?: Date }) {
  if (!timestamp) {
    return <span className="text-foreground-alt/50 text-xs">Not started</span>
  }

  return (
    <span className="text-foreground text-sm font-medium">
      {timestamp.toISOString()}
    </span>
  )
}

// ExecutionLog is the execution's log as a stream filling the pane: it opens
// at its tail and follows new entries while the reader stays at the bottom.
export function ExecutionLog({
  logEntries,
}: {
  logEntries: Execution['logEntries']
}) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const followRef = useRef(true)
  const entries = logEntries ?? []

  // Key each entry by its time, level and text, counting repeats, since
  // entries carry no id and a log only grows.
  const keys = useMemo(() => {
    const seen = new Map<string, number>()
    return (logEntries ?? []).map((entry) => {
      const base = `${entry.timestamp?.getTime() ?? ''}:${entry.level ?? ''}:${entry.message ?? ''}`
      const n = seen.get(base) ?? 0
      seen.set(base, n + 1)
      return `${base}:${n}`
    })
  }, [logEntries])

  // Keep the tail in view as entries arrive, unless the reader scrolled up.
  useEffect(() => {
    const el = scrollRef.current
    if (el && followRef.current) el.scrollTop = el.scrollHeight
  }, [entries.length])

  // An execution with no output yet says so in the pane.
  if (entries.length === 0) {
    return <ViewerStatus state="empty" message="No log output yet." />
  }

  // Follow the tail only while the reader is at the bottom.
  const onScroll = () => {
    const el = scrollRef.current
    if (!el) return
    followRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 8
  }

  return (
    <div
      ref={scrollRef}
      role="log"
      aria-label="Execution log"
      tabIndex={0}
      onScroll={onScroll}
      className="bg-background-dark ps-viewer-pad pe-viewer-pad @pane-roomy:ps-viewer-pad-roomy @pane-roomy:pe-viewer-pad-roomy min-h-0 flex-1 overflow-y-auto py-2"
    >
      {entries.map((entry, index) => (
        <div
          key={keys[index]}
          className="flex gap-2 font-mono text-xs leading-relaxed"
        >
          {entry.level && (
            <span className={logLevelColor(entry.level)}>
              [{entry.level.toUpperCase()}]
            </span>
          )}
          <span className="text-foreground min-w-0 flex-1 break-all whitespace-pre-wrap">
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
  const block = useForgeBlock(objectState, ForgeExecutionTypeID, Execution)
  const execution = block.value ?? undefined

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'logs',
        label: 'Logs',
        stream: true,
        content: <ExecutionLog logEntries={execution?.logEntries} />,
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
        id: 'summary',
        label: 'Summary',
        content: (
          <div className="space-y-3">
            <InfoCard title="Runtime">
              <div className="flex items-center justify-between gap-3">
                <div className="text-foreground-alt/50 text-xs">Started</div>
                <ExecutionTimestamp timestamp={execution?.timestamp} />
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
    [execution],
  )

  // Details lists the execution's identity behind the bar.
  const facts = (
    <>
      {execution?.peerId && (
        <FactRow label="Worker Peer ID" value={execution.peerId} />
      )}
      {execution?.timestamp && (
        <FactRow
          kind="text"
          label="Created"
          value={execution.timestamp.toISOString()}
        />
      )}
      <FactRow label="Object Key" value={objectKey} />
    </>
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuActivity className="size-4" />}
      title="Execution"
      state={execution?.executionState}
      stateLabels={execStateLabels}
      details={{ label: 'Execution', facts }}
      status={forgeBlockStatus('the execution', block)}
      headerStatus={
        execution?.waitingPluginId && (
          <div className="border-foreground/8 text-foreground border-b px-4 py-2 text-xs">
            Waiting for plugin {execution.waitingPluginId} to load
          </div>
        )
      }
      tabs={tabs}
    />
  )
}
