import { useCallback } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { KeyboardKeyDefinition } from './keyboard-map-data.js'
import type { KeybindingCommand } from './keybindings-model.js'
import { modifierDisplayName } from './keyboard-utils.js'

interface KeyboardKeyProps {
  definition: KeyboardKeyDefinition
  mappedCommands: readonly KeybindingCommand[]
  selected: boolean
  hasConflict: boolean
  onHover: (key: string | null) => void
  onSelect: (key: string, commands: readonly KeybindingCommand[]) => void
}

export function KeyboardKey({
  definition,
  mappedCommands,
  selected,
  hasConflict,
  onHover,
  onSelect,
}: KeyboardKeyProps) {
  const hover = useCallback(
    () => onHover(definition.key),
    [definition.key, onHover],
  )
  const clearHover = useCallback(() => onHover(null), [onHover])
  const select = useCallback(
    () => onSelect(definition.key, mappedCommands),
    [definition.key, mappedCommands, onSelect],
  )
  const label = definition.label ?? modifierDisplayName(definition.key)

  return (
    <button
      type="button"
      aria-label={`${label}: ${mappedCommands.length} assigned commands`}
      aria-pressed={selected}
      className={cn(
        'border-foreground/12 bg-background-card-alt text-foreground-alt/60 relative flex h-11 min-w-9 items-center justify-center rounded-md border px-1 font-mono text-[10px] shadow-[0_2px_0_rgba(255,255,255,0.06)] transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/50',
        mappedCommands.length > 0 &&
          'border-brand/30 bg-brand/8 text-foreground hover:border-brand/60 hover:bg-brand/15',
        selected && 'border-brand bg-brand/20 text-brand -translate-y-0.5',
        hasConflict && 'border-destructive/60 bg-destructive/10',
      )}
      style={{ flexGrow: definition.grow ?? 1, flexBasis: 0 }}
      onMouseEnter={hover}
      onMouseLeave={clearHover}
      onFocus={hover}
      onBlur={clearHover}
      onClick={select}
    >
      {label}
      {mappedCommands.length > 0 ? (
        <span
          className={cn(
            'bg-brand text-background absolute -top-1 -right-1 flex size-3.5 items-center justify-center rounded-full text-[8px] font-semibold',
            hasConflict && 'bg-destructive',
          )}
        >
          {mappedCommands.length}
        </span>
      ) : null}
    </button>
  )
}
