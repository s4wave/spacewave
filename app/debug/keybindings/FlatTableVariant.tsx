import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import { LuSearch, LuSlidersHorizontal } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'
import { Input } from '@s4wave/web/ui/input.js'

import { CommandTableRow } from './CommandTableRow.js'
import { commandMatchesQuery } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    pointerEvents: 'none',
    position: 'absolute',
    top: '50%',
    left: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

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
            <LuSlidersHorizontal sx={stylexStyles.rule1} />
            <h2 className="text-sm font-semibold">Shortcut inventory</h2>
          </div>
          <p className="text-foreground-alt/55 mt-1 text-xs">
            Scan every command and record a replacement directly in its row.
          </p>
        </div>
        <label className="relative block w-full sm:max-w-80">
          <LuSearch sx={stylexStyles.rule2} />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Search commands, contexts, or keys"
            aria-label="Search shortcut inventory"
            variant="search"
          />
        </label>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-200 border-collapse">
          <thead>
            <tr className="border-foreground/8 bg-background/30 text-foreground-alt/50 micro-ten border-b text-left font-semibold tracking-wider uppercase">
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
