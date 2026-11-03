import { useCallback } from 'react'

import type { KeyboardKeyDefinition } from './keyboard-map-data.js'
import type { KeybindingCommand } from './keybindings-model.js'
import { modifierDisplayName } from './keyboard-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-brand'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule2: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-brand'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule4: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule5: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-brand'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule6: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: themeColors['--color-brand'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-brand'],
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule7: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule8: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-card-alt'],
    boxShadow: {
      default: '0 2px #ffffff0f',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
    flexBasis: 'var(--keyboard-key-flex-basis)',
    flexGrow: 'var(--keyboard-key-flex-grow)',
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    justifyContent: 'center',
    lineHeight: 1.5,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    position: 'relative',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 11)`,
    minWidth: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: themeColors['--color-brand'],
    color: themeColors['--color-background'],
    display: 'flex',
    fontSize: '8px',
    fontWeight: 'var(--font-weight-semibold)',
    justifyContent: 'center',
    lineHeight: 1.5,
    position: 'absolute',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    right: `calc(${themeLayout['--spacing']} * -1)`,
    top: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule10: { backgroundColor: themeColors['--color-destructive'] },
})

interface KeyboardKeyProps {
  definition: KeyboardKeyDefinition
  mappedCommands: readonly KeybindingCommand[]
  selected: boolean
  hasConflict: boolean
  onHover: (key: string | null) => void
  onSelect: (key: string, commands: readonly KeybindingCommand[]) => void
}

export function KeyboardKey({
  definition,
  mappedCommands,
  selected,
  hasConflict,
  onHover,
  onSelect,
}: KeyboardKeyProps) {
  const hover = useCallback(
    () => onHover(definition.key),
    [definition.key, onHover],
  )
  const clearHover = useCallback(() => onHover(null), [onHover])
  const select = useCallback(
    () => onSelect(definition.key, mappedCommands),
    [definition.key, mappedCommands, onSelect],
  )
  const label = definition.label ?? modifierDisplayName(definition.key)

  return (
    <button
      type="button"
      aria-label={`${label}: ${mappedCommands.length} assigned commands`}
      aria-pressed={selected}
      sx={
        keyStyleStates[mappedCommands.length ? 'assigned' : 'empty'][
          selected ? 'selected' : 'idle'
        ][hasConflict ? 'conflict' : 'clear']
      }
      style={{
        '--keyboard-key-flex-grow': definition.grow ?? 1,
        '--keyboard-key-flex-basis': '0px',
      }}
      onMouseEnter={hover}
      onMouseLeave={clearHover}
      onFocus={hover}
      onBlur={clearHover}
      onClick={select}
    >
      {label}
      {mappedCommands.length > 0 ? (
        <span sx={[stylexStyles.rule9, !!hasConflict && stylexStyles.rule10]}>
          {mappedCommands.length}
        </span>
      ) : null}
    </button>
  )
}

// keyStyleStates names every assignment, selection and conflict appearance.
const keyStyleStates = {
  assigned: {
    selected: { conflict: stylexStyles.rule1, clear: stylexStyles.rule2 },
    idle: { conflict: stylexStyles.rule3, clear: stylexStyles.rule4 },
  },
  empty: {
    selected: { conflict: stylexStyles.rule5, clear: stylexStyles.rule6 },
    idle: { conflict: stylexStyles.rule7, clear: stylexStyles.rule8 },
  },
}
