import { useCallback } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import type { ProcessBindingInfo } from '@s4wave/sdk/space/space.pb.js'

interface ProcessBindingListProps {
  // bindings is the list of process binding info objects.
  bindings: ProcessBindingInfo[]
  // onToggle is called when the user toggles the approval state of a binding.
  // If undefined, the toggle buttons are disabled (not yet connected).
  onToggle?: (objectKey: string, approved: boolean) => void
}

// ProcessBindingList renders a list of process bindings with approve/unapprove toggles.
export function ProcessBindingList({
  bindings,
  onToggle,
}: ProcessBindingListProps) {
  if (bindings.length === 0) {
    return (
      <div className="text-muted-foreground text-sm">
        No process bindings configured.
      </div>
    )
  }

  return (
    <div className="space-y-1">
      {bindings.map((binding) => (
        <ProcessBindingRow
          key={binding.objectKey ?? ''}
          binding={binding}
          onToggle={onToggle}
        />
      ))}
    </div>
  )
}

interface ProcessBindingRowProps {
  binding: ProcessBindingInfo
  onToggle?: (objectKey: string, approved: boolean) => void
}

// ProcessBindingRow renders a single process binding entry.
function ProcessBindingRow({ binding, onToggle }: ProcessBindingRowProps) {
  const objectKey = binding.objectKey ?? ''
  const approved = binding.approved ?? false

  const handleToggle = useCallback(() => {
    onToggle?.(objectKey, !approved)
  }, [onToggle, objectKey, approved])

  return (
    <div className="border-foreground/8 flex items-center justify-between rounded border px-3 py-2">
      <div className="flex flex-col gap-0.5">
        <span className="text-foreground text-sm font-medium">{objectKey}</span>
        {binding.typeId && (
          <span className="text-muted-foreground text-xs">
            {binding.typeId}
          </span>
        )}
        {binding.decidedAt && (
          <span className="text-muted-foreground text-xs">
            {binding.decidedAt.toLocaleString()}
          </span>
        )}
      </div>
      <button
        type="button"
        disabled={!onToggle}
        onClick={handleToggle}
        className={cn(
          'rounded px-2 py-1 text-xs font-medium transition-colors',
          approved ?
            'bg-green-600 text-white hover:bg-green-700'
          : 'bg-neutral-600 text-white hover:bg-neutral-700',
          !onToggle && 'cursor-not-allowed opacity-50',
        )}
      >
        {approved ? 'Approved' : 'Unapproved'}
      </button>
    </div>
  )
}
