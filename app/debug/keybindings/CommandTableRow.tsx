import { useCallback } from 'react'
import { LuTriangleAlert } from 'react-icons/lu'

import { Badge } from '@s4wave/web/ui/badge.js'

import { BindingCapture } from './BindingCapture.js'
import type { KeybindingCommand } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

interface CommandTableRowProps {
  command: KeybindingCommand
  hasConflict: boolean
  isCustomized: boolean
  setBinding: KeybindingVariantProps['setBinding']
  resetBinding: KeybindingVariantProps['resetBinding']
}

export function CommandTableRow({
  command,
  hasConflict,
  isCustomized,
  setBinding,
  resetBinding,
}: CommandTableRowProps) {
  const changeBinding = useCallback(
    (binding: string) => setBinding(command.id, binding),
    [command.id, setBinding],
  )
  const reset = useCallback(
    () => resetBinding(command.id),
    [command.id, resetBinding],
  )

  return (
    <tr className="border-foreground/6 hover:bg-foreground/2 border-b transition-colors last:border-0">
      <td className="min-w-64 px-4 py-3 align-middle">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{command.label}</span>
          {isCustomized ? (
            <span
              className="bg-brand size-1.5 rounded-full"
              title="Customized"
            />
          ) : null}
        </div>
        <p className="text-foreground-alt/55 mt-0.5 text-xs">
          {command.description}
        </p>
      </td>
      <td className="px-4 py-3 align-middle">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="outline" className="border-foreground/10 font-normal">
            {command.category}
          </Badge>
          <Badge variant="secondary" className="bg-foreground/5 font-normal">
            {command.context}
          </Badge>
        </div>
      </td>
      <td className="px-4 py-3 text-right align-middle">
        <div className="flex items-center justify-end gap-2">
          {hasConflict ? (
            <Badge
              variant="destructive"
              className="bg-destructive/15 text-destructive"
            >
              <LuTriangleAlert /> Conflict
            </Badge>
          ) : null}
          <BindingCapture
            binding={command.binding}
            commandLabel={command.label}
            isCustomized={isCustomized}
            onChange={changeBinding}
            onReset={reset}
          />
        </div>
      </td>
    </tr>
  )
}
