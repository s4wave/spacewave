import { useCallback } from 'react'
import {
  LuCheck,
  LuCircleOff,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import { Keycap } from './Keycap.js'
import type { KeybindingCommand } from './keybindings-model.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: { width: '100%', justifyContent: 'flex-start', textAlign: 'left' },
})

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
      aria-pressed={selected}
      variant={selected ? 'selectedRow' : 'unselectedRow'}
      size="row"
      sx={stylexStyles.rule1}
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
        <span className="text-foreground-alt/45 micro-ten mt-0.5 block font-normal">
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
