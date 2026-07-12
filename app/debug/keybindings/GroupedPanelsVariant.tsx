import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import { LuBoxes, LuSearch } from 'react-icons/lu'
import { Input } from '@s4wave/web/ui/input.js'

import { CategoryPanel } from './CategoryPanel.js'
import {
  commandMatchesQuery,
  KEYBINDING_CATEGORIES,
  type KeybindingCategory,
  type KeybindingCommand,
} from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

export function GroupedPanelsVariant(props: KeybindingVariantProps) {
  const { commands } = props
  const [query, setQuery] = useState('')
  const [collapsedCategories, setCollapsedCategories] = useState<
    ReadonlySet<KeybindingCategory>
  >(new Set())

  const groups = useMemo(() => {
    const commandsByCategory = new Map<
      KeybindingCategory,
      KeybindingCommand[]
    >()

    for (const command of commands) {
      if (!commandMatchesQuery(command, query)) continue
      const categoryCommands = commandsByCategory.get(command.category)
      if (categoryCommands) categoryCommands.push(command)
      else commandsByCategory.set(command.category, [command])
    }

    return KEYBINDING_CATEGORIES.flatMap((category) => {
      const categoryCommands = commandsByCategory.get(category)
      return categoryCommands ? [{ category, commands: categoryCommands }] : []
    })
  }, [commands, query])

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )

  const toggleCategory = useCallback((category: KeybindingCategory) => {
    setCollapsedCategories((current) => {
      const next = new Set(current)
      if (next.has(category)) next.delete(category)
      else next.add(category)
      return next
    })
  }, [])

  return (
    <section>
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <LuBoxes className="text-brand size-4" />
            <h2 className="text-sm font-semibold">Collections</h2>
          </div>
          <p className="text-foreground-alt/55 mt-1 max-w-xl text-xs">
            Browse collapsible command families while keeping execution context
            and collisions visible.
          </p>
        </div>
        <label className="relative block w-full sm:max-w-72">
          <LuSearch className="text-foreground-alt/40 pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Filter collections"
            aria-label="Filter shortcut collections"
            className="border-foreground/10 bg-background/30 focus-visible:border-brand/50 focus-visible:ring-brand/15 pl-9"
          />
        </label>
      </div>

      <div className="grid gap-3 xl:grid-cols-2 xl:items-start">
        {groups.map((group) => (
          <CategoryPanel
            key={group.category}
            {...props}
            category={group.category}
            categoryCommands={group.commands}
            collapsed={collapsedCategories.has(group.category)}
            onToggle={toggleCategory}
          />
        ))}
      </div>

      {groups.length === 0 ? (
        <div className="border-foreground/8 text-foreground-alt/55 rounded-lg border border-dashed py-16 text-center text-sm">
          No collections match “{query}”.
        </div>
      ) : null}
    </section>
  )
}
