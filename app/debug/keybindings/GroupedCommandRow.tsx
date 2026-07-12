import { useCallback } from 'react'
import { LuTriangleAlert } from 'react-icons/lu'

import { Badge } from '@s4wave/web/ui/badge.js'

import { BindingCapture } from './BindingCapture.js'
import type { KeybindingCommand } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

interface GroupedCommandRowProps {
  command: KeybindingCommand
  hasConflict: boolean
  isCustomized: boolean
  setBinding: KeybindingVariantProps['setBinding']
  resetBinding: KeybindingVariantProps['resetBinding']
}

export function GroupedCommandRow({
  command,
  hasConflict,
  isCustomized,
  setBinding,
  resetBinding,
}: GroupedCommandRowProps) {
  const changeBinding = useCallback(
    (binding: string) => setBinding(command.id, binding),
    [command.id, setBinding],
  )
  const reset = useCallback(
    () => resetBinding(command.id),
    [command.id, resetBinding],
  )

  return (
    <div className="border-foreground/6 flex flex-col gap-3 border-t px-3.5 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{command.label}</span>
          <Badge
            variant="outline"
            className="border-foreground/10 text-foreground-alt/65 font-normal"
          >
            {command.context}
          </Badge>
          {hasConflict ? (
            <Badge
              variant="destructive"
              className="bg-destructive/15 text-destructive"
            >
              <LuTriangleAlert /> Conflict
            </Badge>
          ) : null}
        </div>
        <p className="text-foreground-alt/50 mt-1 line-clamp-1 text-xs">
          {command.description}
        </p>
      </div>
      <BindingCapture
        binding={command.binding}
        commandLabel={command.label}
        isCustomized={isCustomized}
        className="shrink-0"
        onChange={changeBinding}
        onReset={reset}
      />
    </div>
  )
}
