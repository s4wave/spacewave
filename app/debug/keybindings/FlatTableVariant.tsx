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
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'center' },
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule6: {
    display: 'block',
    position: 'relative',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 80)`,
    },
    width: '100%',
  },
  rule7: { overflowX: 'auto' },
  rule8: {
    borderCollapse: 'collapse',
    minWidth: `calc(${themeLayout['--spacing']} * 200)`,
    width: '100%',
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textAlign: 'left',
    textTransform: 'uppercase',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule10: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'right',
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule13: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 10)`,
    textAlign: 'center',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    pointerEvents: 'none',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    left: `calc(${themeLayout['--spacing']} * 3)`,
    top: '50%',
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
