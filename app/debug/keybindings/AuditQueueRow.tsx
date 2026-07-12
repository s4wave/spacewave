import { useCallback } from 'react'
import { LuCheck, LuCircleOff, LuTriangleAlert } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

interface AuditQueueRowProps {
  command: KeybindingCommand
  selected: boolean
  hasConflict: boolean
  isCustomized: boolean
  onSelect: (commandId: string) => void
}

export function AuditQueueRow({
  command,
  selected,
  hasConflict,
  isCustomized,
  onSelect,
}: AuditQueueRowProps) {
  const select = useCallback(() => onSelect(command.id), [command.id, onSelect])

  return (
    <Button
      type="button"
      variant="ghost"
      aria-pressed={selected}
      className={cn(
        'h-auto w-full justify-start gap-3 rounded-lg border p-3 text-left',
        selected
          ? 'border-brand/30 bg-brand/10 hover:bg-brand/10'
          : 'border-transparent hover:border-foreground/8 hover:bg-foreground/3',
      )}
      onClick={select}
    >
      <span
        className={cn(
          'inline-flex size-8 shrink-0 items-center justify-center rounded-full',
          hasConflict
            ? 'bg-destructive/12 text-destructive'
            : command.binding
              ? 'bg-brand/10 text-brand'
              : 'bg-foreground/5 text-foreground-alt/45',
        )}
      >
        {hasConflict ? (
          <LuTriangleAlert />
        ) : command.binding ? (
          <LuCheck />
        ) : (
          <LuCircleOff />
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{command.label}</span>
          {isCustomized ? (
            <span className="bg-brand size-1.5 shrink-0 rounded-full" />
          ) : null}
        </span>
        <span className="text-foreground-alt/45 mt-0.5 block text-[10px] font-normal">
          {hasConflict
            ? 'Conflicting assignment'
            : command.binding
              ? `${command.category} · ${command.context}`
              : 'No shortcut assigned'}
        </span>
      </span>
      <Keycap chord={command.binding} muted={!selected} />
    </Button>
  )
}
