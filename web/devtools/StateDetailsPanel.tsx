import { useCallback, useMemo } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import {
  useSelectedStatePath,
  useStateDevToolsContext,
} from './StateDevToolsContext.js'
import {
  useStateInspectorValue,
  type StateInspectorEntry,
} from './useStateInspectorEntries.js'

interface StateDetailsPanelProps {
  entry: StateInspectorEntry
}

// StateDetailsPanel displays detailed information about a selected state atom.
export function StateDetailsPanel({ entry }: StateDetailsPanelProps) {
  const devtools = useStateDevToolsContext()
  const selectedPath = useSelectedStatePath()
  const value = useStateInspectorValue(entry)

  const handleCopy = useCallback(() => {
    const targetValue = getValueAtPath(value, selectedPath)
    void navigator.clipboard.writeText(JSON.stringify(targetValue, null, 2))
  }, [selectedPath, value])

  const handleClose = useCallback(() => {
    devtools?.setSelectedAtomId(null)
  }, [devtools])

  const displayValue = useMemo(() => {
    const targetValue = getValueAtPath(value, selectedPath)
    return JSON.stringify(targetValue, null, 2)
  }, [selectedPath, value])

  const pathDisplay =
    selectedPath.length > 0 ? selectedPath.join('.') : '(root)'

  return (
    <div
      className={cn(
        'border-popover-border flex w-64 shrink-0 flex-col gap-2 overflow-auto border-l p-3',
      )}
      data-testid="state-details-panel"
    >
      <div className="flex items-center justify-between">
        <span className="text-text-secondary text-xs font-medium">
          {entry.label}
        </span>
        <button
          type="button"
          onClick={handleClose}
          className="text-foreground-alt hover:text-foreground text-xs"
        >
          Close
        </button>
      </div>

      <div className="flex items-center gap-1">
        <span className="text-foreground-alt text-xs">Path:</span>
        <span className="text-foreground truncate font-mono text-xs">
          {pathDisplay}
        </span>
      </div>

      <div className="flex items-center gap-1">
        <span className="text-foreground-alt text-xs">Scope:</span>
        <span className="text-foreground truncate font-mono text-xs">
          {entry.scope}
        </span>
      </div>

      <div className="flex flex-1 flex-col gap-1 overflow-hidden">
        <span className="text-foreground-alt text-xs">Value:</span>
        <pre
          className={cn(
            'bg-background-deep flex-1 overflow-auto rounded p-2 font-mono text-xs',
            'break-words whitespace-pre-wrap',
          )}
        >
          {displayValue}
        </pre>
      </div>

      <div className="mt-2 flex gap-2">
        <Button size="sm" variant="outline" onClick={handleCopy}>
          Copy
        </Button>
      </div>
    </div>
  )
}

function getValueAtPath(obj: unknown, path: string[]): unknown {
  if (path.length === 0) return obj
  if (obj === null || typeof obj !== 'object') return undefined

  let current: unknown = obj
  for (const key of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return current
}
