import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import { LuSearch, LuSlidersHorizontal } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Input } from '@s4wave/web/ui/input.js'

import { CommandTableRow } from './CommandTableRow.js'
import { commandMatchesQuery } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

export function FlatTableVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [query, setQuery] = useState('')

  const filteredCommands = useMemo(
    () => commands.filter((command) => commandMatchesQuery(command, query)),
    [commands, query],
  )

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )

  return (
    <section className="border-foreground/8 bg-background-card/30 overflow-hidden rounded-xl border shadow-sm">
      <div className="border-foreground/8 flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <LuSlidersHorizontal className="text-brand size-4" />
            <h2 className="text-sm font-semibold">Shortcut inventory</h2>
          </div>
          <p className="text-foreground-alt/55 mt-1 text-xs">
            Scan every command and record a replacement directly in its row.
          </p>
        </div>
        <label className="relative block w-full sm:max-w-80">
          <LuSearch className="text-foreground-alt/40 pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Search commands, contexts, or keys"
            aria-label="Search shortcut inventory"
            className="border-foreground/10 bg-background/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 pl-9"
          />
        </label>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-200 border-collapse">
          <thead>
            <tr className="border-foreground/8 bg-background/30 text-foreground-alt/50 border-b text-left text-[10px] font-semibold tracking-wider uppercase">
              <th className="px-4 py-2.5">Command</th>
              <th className="px-4 py-2.5">Category &amp; context</th>
              <th className="px-4 py-2.5 text-right">Binding</th>
            </tr>
          </thead>
          <tbody>
            {filteredCommands.map((command) => (
              <CommandTableRow
                key={command.id}
                command={command}
                hasConflict={conflictCommandIds.has(command.id)}
                isCustomized={customizedCommandIds.has(command.id)}
                setBinding={setBinding}
                resetBinding={resetBinding}
              />
            ))}
          </tbody>
        </table>
      </div>

      <div
        className={cn(
          'border-foreground/8 text-foreground-alt/55 border-t px-4 py-3 text-xs',
          filteredCommands.length === 0 && 'text-center py-10',
        )}
      >
        {filteredCommands.length === 0
          ? `No commands match “${query}”.`
          : `${filteredCommands.length} of ${commands.length} commands`}
      </div>
    </section>
  )
}
