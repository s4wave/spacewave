import {
  focusContextLabel,
  type KeybindingConflict,
} from './KeybindingResolver.js'

export interface KeybindingConflictListProps {
  conflicts: KeybindingConflict[]
}

export function KeybindingConflictList({
  conflicts,
}: KeybindingConflictListProps) {
  return (
    <div className="border-warning/30 bg-warning/10 rounded border px-3 py-2">
      <div className="text-warning text-xs font-medium">Conflict</div>
      <div className="mt-1 space-y-1">
        {conflicts.map((conflict) => (
          <div
            key={`${conflict.context}:${conflict.kind}:${conflict.key}`}
            className="text-foreground-alt text-xs"
          >
            {focusContextLabel(conflict.context)} {conflict.kind}{' '}
            <span className="font-mono">{conflict.key}</span> is used by{' '}
            {conflict.bindings.map((binding) => binding.label).join(', ')}.
          </div>
        ))}
      </div>
    </div>
  )
}
