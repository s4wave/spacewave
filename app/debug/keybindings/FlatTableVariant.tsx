import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import { LuSearch, LuSlidersHorizontal } from '@s4wave/web/images/icons.js'

import { Input } from '@s4wave/web/ui/input.js'

import { CommandTableRow } from './CommandTableRow.js'
import { commandMatchesQuery } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule2: {
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'center' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule6: {
    position: 'relative',
    display: 'block',
    width: '100%',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 80)`,
    },
  },
  rule7: { overflowX: 'auto' },
  rule8: {
    width: '100%',
    minWidth: `calc(${themeLayout['--spacing']} * 200)`,
    borderCollapse: 'collapse',
  },
  rule9: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    textAlign: 'left',
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule10: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule11: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'right',
  },
  rule12: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule13: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 10)`,
    textAlign: 'center',
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
    <section sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div>
          <div sx={stylexStyles2.rule3}>
            <LuSlidersHorizontal sx={stylexStyles.rule1} />
            <h2 sx={stylexStyles2.rule4}>Shortcut inventory</h2>
          </div>
          <p sx={stylexStyles2.rule5}>
            Scan every command and record a replacement directly in its row.
          </p>
        </div>
        <label sx={stylexStyles2.rule6}>
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

      <div sx={stylexStyles2.rule7}>
        <table sx={stylexStyles2.rule8}>
          <thead>
            <tr sx={stylexStyles2.rule9}>
              <th sx={stylexStyles2.rule10}>Command</th>
              <th sx={stylexStyles2.rule10}>Category &amp; context</th>
              <th sx={stylexStyles2.rule11}>Binding</th>
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
        sx={[
          stylexStyles2.rule12,
          filteredCommands.length === 0 && stylexStyles2.rule13,
        ]}
      >
        {filteredCommands.length === 0
          ? `No commands match “${query}”.`
          : `${filteredCommands.length} of ${commands.length} commands`}
      </div>
    </section>
  )
}
