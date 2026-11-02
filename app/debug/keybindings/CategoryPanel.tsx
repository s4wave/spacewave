import { useCallback, useMemo } from 'react'
import { LuChevronDown } from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'

import { GroupedCommandRow } from './GroupedCommandRow.js'
import type {
  KeybindingCategory,
  KeybindingCommand,
} from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: { rotate: '-90deg' },
})

const stylexStyles = stylex.create({
  rule1: {
    height: 'auto',
    width: '100%',
    justifyContent: 'space-between',
    textAlign: 'left',
  },
})

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
    <article className="border-foreground/8 bg-background-card/30 overflow-hidden rounded-lg border">
      <Button
        type="button"
        variant="category"
        size="category"
        sx={stylexStyles.rule1}
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
            <Badge variant="conflict">{conflictCount} conflicting</Badge>
          ) : null}
          <LuChevronDown
            sx={[stylexStyles2.rule1, !!collapsed && stylexStyles2.rule2]}
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
