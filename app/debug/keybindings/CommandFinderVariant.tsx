import {
  useCallback,
  useId,
  useMemo,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from 'react'
import {
  LuCornerDownLeft,
  LuSearch,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Input } from '@s4wave/web/ui/input.js'

import { BindingCapture } from './BindingCapture.js'
import { FinderResult } from './FinderResult.js'
import { Keycap } from './Keycap.js'
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
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-5xl)' },
  rule2: {
    '--tw-shadow':
      '0 20px 25px -5px var(--tw-shadow-color, #0000001a), 0 8px 10px -6px var(--tw-shadow-color, #0000001a)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 40%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    display: { default: 'none', '@media (min-width: 40rem)': 'inline-flex' },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'minmax(18rem, .8fr) minmax(24rem, 1.2fr)',
    },
    minHeight: `calc(${themeLayout['--spacing']} * 120)`,
  },
  rule6: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      '@media (min-width: 64rem)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', '@media (min-width: 64rem)': 0 },
    borderRightStyle: {
      default: null,
      '@media (min-width: 64rem)': themeUtilityState['--tw-border-style'],
    },
    borderRightWidth: { default: null, '@media (min-width: 64rem)': '1px' },
    maxHeight: `calc(${themeLayout['--spacing']} * 145)`,
    overflowY: 'auto',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    justifyContent: 'space-between',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule8: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 16)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule9: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 7)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
  },
  rule10: { position: 'sticky', top: `calc(${themeLayout['--spacing']} * 5)` },
  rule11: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    color: themeColors['--color-brand'],
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule12: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule14: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule15: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card-alt']} 50%, transparent)`,
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule16: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule17: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule18: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule19: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    fontSize: themeFonts['--text-xs'],
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule20: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule21: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    display: 'block',
  },
  rule22: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule23: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 75%, transparent)`,
    display: 'block',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export function CommandFinderVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    commands[0]?.id ?? '',
  )
  const listboxId = `${useId()}-results`
  const focusSearchInput = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const results = useMemo(
    () => commands.filter((command) => commandMatchesQuery(command, query)),
    [commands, query],
  )
  const selectedCommand = useMemo(
    () =>
      results.find((command) => command.id === selectedCommandId) ?? results[0],
    [results, selectedCommandId],
  )

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value
      setQuery(value)
    },
    [],
  )

  const handleSearchKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (results.length === 0) return
      const currentIndex = results.findIndex(
        (command) => command.id === selectedCommand?.id,
      )

      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setSelectedCommandId(results[(currentIndex + 1) % results.length].id)
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        const nextIndex =
          currentIndex <= 0 ? results.length - 1 : currentIndex - 1
        setSelectedCommandId(results[nextIndex].id)
      } else if (event.key === 'Enter') {
        event.preventDefault()
        setSelectedCommandId(results[Math.max(currentIndex, 0)].id)
      }
    },
    [results, selectedCommand?.id],
  )

  const changeBinding = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )
  const reset = useCallback(() => {
    if (selectedCommand) resetBinding(selectedCommand.id)
  }, [resetBinding, selectedCommand])

  return (
    <section sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuSearch sx={stylexStyles.rule1} />
          <Input
            ref={focusSearchInput}
            value={query}
            onChange={handleQueryChange}
            onKeyDown={handleSearchKeyDown}
            placeholder="Type a command, context, or shortcut…"
            aria-label="Find a command to rebind"
            role="combobox"
            aria-autocomplete="list"
            aria-controls={listboxId}
            aria-expanded
            aria-activedescendant={
              selectedCommand
                ? `${listboxId}-option-${selectedCommand.id}`
                : undefined
            }
            variant="command"
          />
          <span sx={stylexStyles2.rule4}>↑↓ navigate</span>
        </div>

        <div sx={stylexStyles2.rule5}>
          <div sx={stylexStyles2.rule6}>
            <div sx={stylexStyles2.rule7}>
              <span>Matches</span>
              <span>{results.length}</span>
            </div>
            <div
              id={listboxId}
              role="listbox"
              aria-label="Matching commands"
              sx={false}
            >
              {results.map((command) => (
                <FinderResult
                  sx={nativeChildSpacing.s1}
                  key={command.id}
                  id={`${listboxId}-option-${command.id}`}
                  command={command}
                  selected={command.id === selectedCommand?.id}
                  hasConflict={conflictCommandIds.has(command.id)}
                  onSelect={setSelectedCommandId}
                />
              ))}
            </div>
            {results.length === 0 ? (
              <div sx={stylexStyles2.rule8}>No commands match “{query}”.</div>
            ) : null}
          </div>

          <aside sx={stylexStyles2.rule9}>
            {selectedCommand ? (
              <div sx={stylexStyles2.rule10}>
                <span sx={stylexStyles2.rule11}>Focused binding editor</span>
                <h2 sx={stylexStyles2.rule12}>{selectedCommand.label}</h2>
                <p sx={stylexStyles2.rule13}>{selectedCommand.description}</p>

                <div sx={stylexStyles2.rule14}>
                  <Badge variant="muted">{selectedCommand.category}</Badge>
                  <Badge variant="subtle">{selectedCommand.context}</Badge>
                  {conflictCommandIds.has(selectedCommand.id) ? (
                    <Badge variant="conflict">
                      <LuTriangleAlert /> Conflicting assignment
                    </Badge>
                  ) : null}
                </div>

                <div sx={stylexStyles2.rule15}>
                  <span sx={stylexStyles2.rule16}>Active shortcut</span>
                  <BindingCapture
                    binding={selectedCommand.binding}
                    commandLabel={selectedCommand.label}
                    isCustomized={customizedCommandIds.has(selectedCommand.id)}
                    sx={stylexStyles2.rule17}
                    onChange={changeBinding}
                    onReset={reset}
                  />
                  <div sx={stylexStyles2.rule18}>
                    <LuCornerDownLeft sx={stylexStyles.rule2} />
                    Click the binding, then press a new key combination.
                  </div>
                </div>

                <div sx={stylexStyles2.rule19}>
                  <div sx={stylexStyles2.rule20}>
                    <span sx={stylexStyles2.rule21}>Default</span>
                    <Keycap
                      chord={selectedCommand.defaultBinding}
                      sx={stylexStyles2.rule22}
                      muted
                    />
                  </div>
                  <div sx={stylexStyles2.rule20}>
                    <span sx={stylexStyles2.rule21}>Command ID</span>
                    <code sx={stylexStyles2.rule23}>{selectedCommand.id}</code>
                  </div>
                </div>
              </div>
            ) : null}
          </aside>
        </div>
      </div>
    </section>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
