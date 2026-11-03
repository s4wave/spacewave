import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import {
  LuKeyboard,
  LuMousePointer2,
  LuSearch,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import { BindingCapture } from './BindingCapture.js'
import { KeyboardKey } from './KeyboardKey.js'
import { Keycap } from './Keycap.js'
import { commandUsesKey, keyboardRowsForPlatform } from './keyboard-map-data.js'
import {
  commandMatchesQuery,
  type KeybindingCommand,
} from './keybindings-model.js'
import { currentKeybindingPlatform } from './keyboard-utils.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    marginInline: 'auto',
    maxWidth: 'var(--container-4xl)',
    minWidth: `calc(${themeLayout['--spacing']} * 180)`,
  },
  rule2: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
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
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 80rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule7: {
    overflowX: 'auto',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
  },
  rule8: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule11: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
  },
  rule12: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
  rule13: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule15: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    textTransform: 'uppercase',
  },
  rule16: {
    marginTop: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule17: { minWidth: 0 },
  rule18: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule19: {
    display: 'block',
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule20: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule21: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule22: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule23: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule24: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles2 = stylex.create({
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
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: 'auto',
    width: '100%',
    justifyContent: 'space-between',
    textAlign: 'left',
  },
})

export function KeyboardMapVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    () =>
      commands.find((command) => command.binding)?.id ?? commands[0]?.id ?? '',
  )
  const [inspectedKey, setInspectedKey] = useState<string | null>('K')
  const [hoveredKey, setHoveredKey] = useState<string | null>(null)
  const platform = currentKeybindingPlatform()
  const keyboardRows = keyboardRowsForPlatform(platform)

  const visibleCommands = useMemo(
    () => commands.filter((command) => commandMatchesQuery(command, query)),
    [commands, query],
  )
  const activeKey = hoveredKey ?? inspectedKey
  const activeKeyCommands = useMemo(
    () =>
      activeKey
        ? visibleCommands.filter((command) =>
            commandUsesKey(command, activeKey, platform),
          )
        : [],
    [activeKey, platform, visibleCommands],
  )
  const selectedCommand = useMemo(
    () =>
      activeKeyCommands.find((command) => command.id === selectedCommandId) ??
      activeKeyCommands[0],
    [activeKeyCommands, selectedCommandId],
  )

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )
  const inspectKey = useCallback(
    (key: string, mappedCommands: readonly KeybindingCommand[]) => {
      setInspectedKey(key)
      setSelectedCommandId(mappedCommands[0]?.id ?? '')
    },
    [],
  )
  const selectCommand = useCallback((commandId: string) => {
    setSelectedCommandId(commandId)
  }, [])
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
    <section sx={false}>
      <div sx={[nativeChildSpacing.s4, stylexStyles3.rule1]}>
        <div>
          <div sx={stylexStyles3.rule2}>
            <LuKeyboard sx={stylexStyles2.rule1} />
            <h2 sx={stylexStyles3.rule3}>Keyboard atlas</h2>
          </div>
          <p sx={stylexStyles3.rule4}>
            Hover to inspect assignments. Click a highlighted key, choose its
            command, then record a replacement.
          </p>
        </div>
        <label sx={stylexStyles3.rule5}>
          <LuSearch sx={stylexStyles2.rule2} />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Highlight matching commands"
            aria-label="Filter keyboard assignments"
            variant="searchPanel"
          />
        </label>
      </div>

      <div sx={[nativeChildSpacing.s4, stylexStyles3.rule6]}>
        <div sx={stylexStyles3.rule7}>
          <div sx={stylexStyles4.rule1}>
            {keyboardRows.map((row) => (
              <div
                key={row.id}
                sx={[nativeChildSpacing.s2, stylexStyles3.rule8]}
              >
                {row.keys.map((definition) => {
                  const mappedCommands = visibleCommands.filter((command) =>
                    commandUsesKey(command, definition.key, platform),
                  )
                  const hasConflict = mappedCommands.some((command) =>
                    conflictCommandIds.has(command.id),
                  )
                  return (
                    <KeyboardKey
                      key={definition.side ?? definition.key}
                      definition={definition}
                      mappedCommands={mappedCommands}
                      selected={activeKey === definition.key}
                      hasConflict={hasConflict}
                      onHover={setHoveredKey}
                      onSelect={inspectKey}
                    />
                  )
                })}
              </div>
            ))}
          </div>
          <div sx={stylexStyles3.rule9}>
            <span sx={stylexStyles3.rule10}>
              <span sx={stylexStyles3.rule11} />
              Assigned
            </span>
            <span sx={stylexStyles3.rule10}>
              <span sx={stylexStyles3.rule12} />
              Conflict
            </span>
            <span>{visibleCommands.length} commands represented</span>
          </div>
        </div>

        <aside sx={stylexStyles3.rule13}>
          <div sx={stylexStyles3.rule14}>
            <div>
              <span sx={stylexStyles3.rule15}>Inspected key</span>
              <div sx={stylexStyles3.rule16}>{activeKey ?? '—'}</div>
            </div>
            <LuMousePointer2 sx={stylexStyles2.rule3} />
          </div>

          <div sx={stylexStyles4.rule2}>
            {activeKeyCommands.map((command) => (
              <Button
                key={command.id}
                type="button"
                variant={
                  command.id === selectedCommand?.id ? 'brandGhost' : 'ghost'
                }
                size="commandRow"
                sx={[nativeChildSpacing.s1, stylexStyles.rule1]}
                aria-pressed={command.id === selectedCommand?.id}
                onClick={() => selectCommand(command.id)}
              >
                <span sx={stylexStyles3.rule17}>
                  <span sx={stylexStyles3.rule18}>{command.label}</span>
                  <span sx={stylexStyles3.rule19}>{command.context}</span>
                </span>
                <Keycap chord={command.binding} muted />
              </Button>
            ))}
            {activeKeyCommands.length === 0 ? (
              <p sx={[nativeChildSpacing.s1, stylexStyles3.rule20]}>
                No matching command uses this key.
              </p>
            ) : null}
          </div>

          {selectedCommand ? (
            <div sx={stylexStyles3.rule21}>
              <div sx={stylexStyles3.rule22}>
                <div>
                  <h3 sx={stylexStyles3.rule3}>{selectedCommand.label}</h3>
                  <p sx={stylexStyles3.rule23}>{selectedCommand.description}</p>
                </div>
                {conflictCommandIds.has(selectedCommand.id) ? (
                  <Badge variant="conflict">
                    <LuTriangleAlert />
                  </Badge>
                ) : null}
              </div>
              <BindingCapture
                binding={selectedCommand.binding}
                commandLabel={selectedCommand.label}
                isCustomized={customizedCommandIds.has(selectedCommand.id)}
                sx={stylexStyles3.rule24}
                onChange={changeBinding}
                onReset={reset}
              />
            </div>
          ) : null}
        </aside>
      </div>
    </section>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
