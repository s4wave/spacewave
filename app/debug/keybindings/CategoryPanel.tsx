import { useCallback, useMemo } from 'react'
import { LuChevronDown } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'

import { GroupedCommandRow } from './GroupedCommandRow.js'
import type {
  KeybindingCategory,
  KeybindingCommand,
} from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

interface CategoryPanelProps extends KeybindingVariantProps {
  category: KeybindingCategory
  categoryCommands: readonly KeybindingCommand[]
  collapsed: boolean
  onToggle: (category: KeybindingCategory) => void
}

export function CategoryPanel({
  category,
  categoryCommands,
  collapsed,
  onToggle,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: CategoryPanelProps) {
  const toggle = useCallback(() => onToggle(category), [category, onToggle])
  const conflictCount = useMemo(
    () =>
      categoryCommands.filter((command) => conflictCommandIds.has(command.id))
        .length,
    [categoryCommands, conflictCommandIds],
  )
  const contexts = useMemo(
    () => [...new Set(categoryCommands.map((command) => command.context))],
    [categoryCommands],
  )

  return (
    <article className="border-foreground/8 bg-background-card/30 overflow-hidden rounded-lg border backdrop-blur-sm">
      <Button
        type="button"
        variant="ghost"
        className="hover:bg-foreground/3 h-auto w-full justify-between rounded-none px-3.5 py-3 text-left"
        aria-expanded={!collapsed}
        onClick={toggle}
      >
        <span className="min-w-0">
          <span className="flex items-center gap-2 text-sm font-semibold">
            <span className="bg-brand/10 text-brand inline-flex size-7 items-center justify-center rounded-md text-xs">
              {category.slice(0, 2).toLocaleUpperCase()}
            </span>
            {category}
            <span className="text-foreground-alt/40 text-xs font-normal">
              {categoryCommands.length}
            </span>
          </span>
          <span className="mt-2 flex flex-wrap gap-1.5">
            {contexts.map((context) => (
              <span
                key={context}
                className="bg-foreground/5 text-foreground-alt/55 rounded px-1.5 py-0.5 text-xs font-normal"
              >
                {context}
              </span>
            ))}
          </span>
        </span>
        <span className="flex items-center gap-2">
          {conflictCount > 0 ? (
            <Badge
              variant="destructive"
              className="bg-destructive/15 text-destructive"
            >
              {conflictCount} conflicting
            </Badge>
          ) : null}
          <LuChevronDown
            className={cn(
              'text-foreground-alt/50 size-4 transition-transform',
              collapsed && '-rotate-90',
            )}
          />
        </span>
      </Button>
      {collapsed
        ? null
        : categoryCommands.map((command) => (
            <GroupedCommandRow
              key={command.id}
              command={command}
              hasConflict={conflictCommandIds.has(command.id)}
              isCustomized={customizedCommandIds.has(command.id)}
              setBinding={setBinding}
              resetBinding={resetBinding}
            />
          ))}
    </article>
  )
}
