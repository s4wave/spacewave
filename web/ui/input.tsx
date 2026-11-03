import { buttonGroupControlStyles } from '../style/button-group.stylex.js'
import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  base: {
    '--tw-outline-style': 'none',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    paddingBlock: themeLayout['--spacing'],
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: 'none',
    pointerEvents: { default: null, ':disabled': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'color, box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: 0,
    width: '100%',
    '::file-selector-button': {
      '--tw-font-weight': themeFonts['--font-weight-medium'],
      borderStyle: themeUtilityState['--tw-border-style'],
      borderWidth: 0,
      backgroundColor: '#0000',
      color: themeColors['--color-foreground'],
      display: 'inline-flex',
      fontSize: themeFonts['--text-sm'],
      fontWeight: themeFonts['--font-weight-medium'],
      lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      height: `calc(${themeLayout['--spacing']} * 7)`,
    },
    '::selection': {
      backgroundColor: themeColors['--color-primary'],
      color: themeColors['--color-primary-foreground'],
    },
  },
  default: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  mono: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  wizardError: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  compact: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  toolbar: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 7)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  plugin: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  displayName: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    fontSize: {
      default: themeFonts['--text-sm'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  command: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(0px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 0 #0000',
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: 0,
    paddingInline: 0,
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: 'auto',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  searchPanel: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  search: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  config: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    height: `calc(${themeLayout['--spacing']} * 9)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
})

const variantStyles = {
  config: styles.config,
  configCompact: styles.compact,
  search: styles.search,
  searchPanel: styles.searchPanel,
  command: styles.command,
  displayName: styles.displayName,
  plugin: styles.plugin,
  toolbar: styles.toolbar,
  configMono: styles.mono,
  wizard: styles.compact,
  wizardError: styles.wizardError,
  wizardMono: styles.mono,
}

/** InputProps supplies native field behavior and a compiled style override. */
interface InputProps extends Omit<React.ComponentProps<'input'>, 'className'> {
  className?: never
  sx?: ComponentStyles
  variant?: keyof typeof variantStyles | null
}

const groupClass = stylex.props([
  buttonGroupControlStyles.child,
  buttonGroupControlStyles.input,
]).className

/** Input renders a themed native field with compiled caller styles. */
function Input({ sx, type, variant, ...props }: InputProps) {
  // Preserve field defaults while applying the group's direct-child rules independently.
  const attributes = stylexProps(
    [styles.base, variant ? variantStyles[variant] : styles.default, sx],
    props,
  )
  const inheritedClass = useControlStyles()
  attributes.className = [attributes.className, groupClass, inheritedClass]
    .filter(Boolean)
    .join(' ')
  return <input type={type} data-slot="input" {...attributes} />
}

export { Input }
