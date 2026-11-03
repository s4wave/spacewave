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

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule2: { minWidth: 0 },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { rotate: '-90deg' },
})

const stylexStyles = stylex.create({
  rule1: {
    justifyContent: 'space-between',
    textAlign: 'left',
    height: 'auto',
    width: '100%',
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
    <article sx={stylexStyles3.rule1}>
      <Button
        type="button"
        variant="category"
        size="category"
        sx={stylexStyles.rule1}
        aria-expanded={!collapsed}
        onClick={toggle}
      >
        <span sx={stylexStyles3.rule2}>
          <span sx={stylexStyles3.rule3}>
            <span sx={stylexStyles3.rule4}>
              {category.slice(0, 2).toLocaleUpperCase()}
            </span>
            {category}
            <span sx={stylexStyles3.rule5}>{categoryCommands.length}</span>
          </span>
          <span sx={stylexStyles3.rule6}>
            {contexts.map((context) => (
              <span key={context} sx={stylexStyles3.rule7}>
                {context}
              </span>
            ))}
          </span>
        </span>
        <span sx={stylexStyles3.rule8}>
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
