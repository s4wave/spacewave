import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import { LuBoxes, LuSearch } from '@s4wave/web/images/icons.js'
import { Input } from '@s4wave/web/ui/input.js'

import { CategoryPanel } from './CategoryPanel.js'
import {
  commandMatchesQuery,
  KEYBINDING_CATEGORIES,
  type KeybindingCategory,
  type KeybindingCommand,
} from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'flex-end' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    maxWidth: 'var(--container-xl)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule5: {
    position: 'relative',
    display: 'block',
    width: '100%',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 72)`,
    },
  },
  rule6: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 80rem)': 'repeat(2, minmax(0, 1fr))',
    },
    alignItems: { default: null, '@media (min-width: 80rem)': 'flex-start' },
  },
  rule7: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 16)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
})

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
      <div sx={stylexStyles2.rule1}>
        <div>
          <div sx={stylexStyles2.rule2}>
            <LuBoxes sx={stylexStyles.rule1} />
            <h2 sx={stylexStyles2.rule3}>Collections</h2>
          </div>
          <p sx={stylexStyles2.rule4}>
            Browse collapsible command families while keeping execution context
            and collisions visible.
          </p>
        </div>
        <label sx={stylexStyles2.rule5}>
          <LuSearch sx={stylexStyles.rule2} />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Filter collections"
            aria-label="Filter shortcut collections"
            variant="searchPanel"
          />
        </label>
      </div>

      <div sx={stylexStyles2.rule6}>
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
        <div sx={stylexStyles2.rule7}>No collections match “{query}”.</div>
      ) : null}
    </section>
  )
}
