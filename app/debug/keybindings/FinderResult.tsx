import { useCallback } from 'react'
import { LuCheck, LuCommand, LuTriangleAlert } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

interface FinderResultProps {
  id: string
  command: KeybindingCommand
  selected: boolean
  hasConflict: boolean
  onSelect: (commandId: string) => void
}

export function FinderResult({
  id,
  command,
  selected,
  hasConflict,
  onSelect,
}: FinderResultProps) {
  const select = useCallback(() => onSelect(command.id), [command.id, onSelect])

  return (
    <Button
      id={id}
      role="option"
      aria-selected={selected}
      type="button"
      tabIndex={-1}
      variant="ghost"
      className={cn(
        'h-auto w-full justify-start rounded-lg px-3 py-2.5 text-left',
        selected
          ? 'border-brand/20 bg-brand/10 hover:bg-brand/10 border'
          : 'border border-transparent hover:bg-foreground/4',
      )}
      onClick={select}
    >
      <span
        className={cn(
          'inline-flex size-7 shrink-0 items-center justify-center rounded-md',
          selected
            ? 'bg-brand/15 text-brand'
            : 'bg-foreground/5 text-foreground-alt/50',
        )}
      >
        {selected ? <LuCheck /> : <LuCommand />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{command.label}</span>
          {hasConflict ? (
            <LuTriangleAlert className="text-destructive size-3.5 shrink-0" />
          ) : null}
        </span>
        <span className="text-foreground-alt/45 mt-0.5 block truncate text-[11px] font-normal">
          {command.category} · {command.context}
        </span>
      </span>
      <Keycap chord={command.binding} muted={!selected} />
    </Button>
  )
}
