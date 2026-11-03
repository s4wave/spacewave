import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import {
  LuChartNoAxesColumnIncreasing,
  LuCheck,
  LuSearch,
  LuSparkles,
  LuTriangleAlert,
  LuWandSparkles,
} from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import { AuditQueueRow } from './AuditQueueRow.js'
import { BindingCapture } from './BindingCapture.js'
import { Keycap } from './Keycap.js'
import { commandMatchesQuery, contextsOverlap } from './keybindings-model.js'
import { chordsMatch } from './keyboard-utils.js'
import type { KeybindingVariantProps } from './variant-types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  motion as themeMotion,
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    maxHeight: `calc(${themeLayout['--spacing']} * 135)`,
    overflowY: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule3: {
    marginTop: themeLayout['--spacing'],
    maxWidth: 'var(--container-2xl)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule4: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule5: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 35%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'block',
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 35%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule10: {
    display: 'grid',
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'minmax(16rem, .8fr) minmax(24rem, 1.2fr)',
    },
  },
  rule11: {
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      '@media (min-width: 64rem)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', '@media (min-width: 64rem)': 0 },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRightStyle: {
      default: null,
      '@media (min-width: 64rem)': themeUtilityState['--tw-border-style'],
    },
    borderRightWidth: { default: null, '@media (min-width: 64rem)': '1px' },
  },
  rule12: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule13: { position: 'relative', display: 'block' },
  rule14: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 16)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule15: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    padding: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 7)`,
    },
  },
  rule16: {
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'flex-start' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
  },
  rule17: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule18: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule19: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    maxWidth: 'var(--container-xl)',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule20: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule21: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule22: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    textTransform: 'uppercase',
  },
  rule23: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 25%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule25: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule26: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
      },
    },
  },
  rule27: {
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule28: {
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule29: {
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule30: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule31: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
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
  rule4: { color: themeColors['--color-destructive'] },
  rule5: { color: themeColors['--color-brand'] },
})

type AuditMode = 'issues' | 'customized' | 'all'

const SUGGESTED_CHORDS = [
  'CmdOrCtrl+Shift+1',
  'CmdOrCtrl+Shift+2',
  'CmdOrCtrl+Shift+3',
  'Ctrl+Alt+K',
  'Ctrl+Alt+L',
  'Alt+Shift+K',
  'Alt+Shift+L',
  'CmdOrCtrl+Alt+K',
  'CmdOrCtrl+Alt+L',
] as const

export function ShortcutAuditVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [mode, setMode] = useState<AuditMode>('issues')
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    () =>
      commands.find((command) => conflictCommandIds.has(command.id))?.id ??
      commands.find((command) => !command.binding)?.id ??
      commands[0]?.id ??
      '',
  )

  const unassignedCount = useMemo(
    () => commands.filter((command) => !command.binding).length,
    [commands],
  )
  const queueCommands = useMemo(
    () =>
      commands.filter((command) => {
        if (!commandMatchesQuery(command, query)) return false
        if (mode === 'issues') {
          return conflictCommandIds.has(command.id) || !command.binding
        }
        if (mode === 'customized') return customizedCommandIds.has(command.id)
        return true
      }),
    [commands, conflictCommandIds, customizedCommandIds, mode, query],
  )
  const selectedCommand = useMemo(
    () =>
      queueCommands.find((command) => command.id === selectedCommandId) ??
      queueCommands[0],
    [queueCommands, selectedCommandId],
  )
  const conflictPartners = useMemo(() => {
    if (!selectedCommand?.binding) return []
    return commands.filter(
      (command) =>
        command.id !== selectedCommand.id &&
        chordsMatch(command.binding, selectedCommand.binding) &&
        contextsOverlap(command.context, selectedCommand.context),
    )
  }, [commands, selectedCommand])
  const suggestions = useMemo(() => {
    if (!selectedCommand) return []
    return SUGGESTED_CHORDS.filter((chord) =>
      commands.every(
        (command) =>
          !chordsMatch(command.binding, chord) ||
          !contextsOverlap(command.context, selectedCommand.context),
      ),
    ).slice(0, 3)
  }, [commands, selectedCommand])

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )
  const showIssues = useCallback(() => setMode('issues'), [])
  const showCustomized = useCallback(() => setMode('customized'), [])
  const showAll = useCallback(() => setMode('all'), [])
  const changeBinding = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )
  const reset = useCallback(() => {
    if (selectedCommand) resetBinding(selectedCommand.id)
  }, [resetBinding, selectedCommand])
  const applySuggestion = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )

  return (
    <section sx={false}>
      <div sx={nativeChildSpacing.s4}>
        <div sx={stylexStyles2.rule1}>
          <LuChartNoAxesColumnIncreasing sx={stylexStyles.rule1} />
          <h2 sx={stylexStyles2.rule2}>Binding health workbench</h2>
        </div>
        <p sx={stylexStyles2.rule3}>
          Triage collisions and missing shortcuts, compare defaults, and apply a
          clear alternative without leaving the review queue.
        </p>
      </div>

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule4]}>
        <button
          type="button"
          aria-pressed={mode === 'issues'}
          sx={[stylexStyles2.rule5, mode === 'issues' && stylexStyles2.rule6]}
          onClick={showIssues}
        >
          <LuTriangleAlert sx={stylexStyles.rule2} />
          <strong sx={stylexStyles2.rule7}>
            {conflictCommandIds.size + unassignedCount}
          </strong>
          <span sx={stylexStyles2.rule8}>Needs attention</span>
        </button>
        <button
          type="button"
          aria-pressed={mode === 'customized'}
          sx={[
            stylexStyles2.rule5,
            mode === 'customized' && stylexStyles2.rule9,
          ]}
          onClick={showCustomized}
        >
          <LuSparkles sx={stylexStyles.rule1} />
          <strong sx={stylexStyles2.rule7}>{customizedCommandIds.size}</strong>
          <span sx={stylexStyles2.rule8}>Customized</span>
        </button>
        <button
          type="button"
          aria-pressed={mode === 'all'}
          sx={[stylexStyles2.rule5, mode === 'all' && stylexStyles2.rule9]}
          onClick={showAll}
        >
          <LuCheck sx={stylexStyles.rule1} />
          <strong sx={stylexStyles2.rule7}>
            {commands.length - conflictCommandIds.size - unassignedCount}
          </strong>
          <span sx={stylexStyles2.rule8}>Ready to use</span>
        </button>
      </div>

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule10]}>
        <div sx={stylexStyles2.rule11}>
          <div sx={stylexStyles2.rule12}>
            <label sx={stylexStyles2.rule13}>
              <LuSearch sx={stylexStyles.rule3} />
              <Input
                value={query}
                onChange={handleQueryChange}
                placeholder="Search review queue"
                aria-label="Search binding review queue"
                variant="searchPanel"
              />
            </label>
          </div>
          <div sx={stylexStyles4.rule1}>
            {queueCommands.map((command) => (
              <AuditQueueRow
                sx={nativeChildSpacingNext.s1}
                key={command.id}
                command={command}
                selected={command.id === selectedCommand?.id}
                hasConflict={conflictCommandIds.has(command.id)}
                isCustomized={customizedCommandIds.has(command.id)}
                onSelect={setSelectedCommandId}
              />
            ))}
            {queueCommands.length === 0 ? (
              <div sx={[nativeChildSpacingNext.s1, stylexStyles2.rule14]}>
                No commands in this view match “{query}”.
              </div>
            ) : null}
          </div>
        </div>

        <aside sx={stylexStyles2.rule15}>
          {selectedCommand ? (
            <div>
              <div sx={stylexStyles2.rule16}>
                <div>
                  <span sx={stylexStyles2.rule17}>Review assignment</span>
                  <h3 sx={stylexStyles2.rule18}>{selectedCommand.label}</h3>
                  <p sx={stylexStyles2.rule19}>{selectedCommand.description}</p>
                </div>
                <Badge variant="muted">{selectedCommand.context}</Badge>
              </div>

              <div sx={stylexStyles2.rule20}>
                <div sx={stylexStyles2.rule21}>
                  <span sx={stylexStyles2.rule22}>Current</span>
                  <BindingCapture
                    binding={selectedCommand.binding}
                    commandLabel={selectedCommand.label}
                    isCustomized={customizedCommandIds.has(selectedCommand.id)}
                    sx={stylexStyles2.rule23}
                    onChange={changeBinding}
                    onReset={reset}
                  />
                </div>
                <div sx={stylexStyles2.rule21}>
                  <span sx={stylexStyles2.rule22}>Factory default</span>
                  <Keycap
                    chord={selectedCommand.defaultBinding}
                    sx={stylexStyles2.rule23}
                    muted
                  />
                </div>
              </div>

              {conflictPartners.length > 0 ? (
                <div sx={stylexStyles2.rule24}>
                  <div sx={stylexStyles2.rule25}>
                    <LuTriangleAlert sx={stylexStyles.rule4} />
                    Also assigned in an overlapping context
                  </div>
                  <div sx={stylexStyles3.rule1}>
                    {conflictPartners.map((command) => (
                      <button
                        key={command.id}
                        type="button"
                        sx={[nativeChildSpacing.s2, stylexStyles2.rule26]}
                        onClick={() => setSelectedCommandId(command.id)}
                      >
                        <span>
                          {command.label}
                          <span sx={stylexStyles2.rule27}>
                            {command.context}
                          </span>
                        </span>
                        <Keycap chord={command.binding} muted />
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                <div sx={stylexStyles2.rule28}>
                  <LuCheck sx={stylexStyles.rule1} />
                  This assignment is clear in its active context.
                </div>
              )}

              <div sx={stylexStyles2.rule29}>
                <div sx={stylexStyles2.rule25}>
                  <LuWandSparkles sx={stylexStyles.rule5} />
                  Available alternatives
                </div>
                <p sx={stylexStyles2.rule30}>
                  These combinations are unused in {selectedCommand.context}.
                </p>
                <div sx={stylexStyles2.rule31}>
                  {suggestions.map((binding) => (
                    <Button
                      key={binding}
                      type="button"
                      variant="debugOutline"
                      onClick={() => applySuggestion(binding)}
                    >
                      <Keycap chord={binding} />
                    </Button>
                  ))}
                </div>
              </div>
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
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
