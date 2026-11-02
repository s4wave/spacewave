import { cn } from '@s4wave/web/style/utils.js'
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
    width: '100%',
    minWidth: 0,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    paddingBlock: themeLayout['--spacing'],
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    transitionProperty: 'color, box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::file-selector-button': {
      display: 'inline-flex',
      height: `calc(${themeLayout['--spacing']} * 7)`,
      borderStyle: themeUtilityState['--tw-border-style'],
      borderWidth: 0,
      backgroundColor: '#0000',
      fontSize: themeFonts['--text-sm'],
      lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '--tw-font-weight': themeFonts['--font-weight-medium'],
      fontWeight: themeFonts['--font-weight-medium'],
      color: themeColors['--color-foreground'],
    },
    pointerEvents: { default: null, ':disabled': 'none' },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    '::selection': {
      backgroundColor: themeColors['--color-primary'],
      color: themeColors['--color-primary-foreground'],
    },
  },
  default: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  mono: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  wizardError: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  compact: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  toolbar: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  plugin: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  displayName: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: themeFonts['--text-sm'],
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  command: {
    height: 'auto',
    borderWidth: 0,
    borderColor: {
      default: themeColors['--color-input'],
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: '#0000',
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: 0,
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    '--tw-shadow': '0 0 #0000',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(0px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  searchPanel: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  search: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  config: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      ':is([aria-invalid="true"])': themeColors['--color-destructive'],
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-input']} 30%, transparent)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: 'var(--text-base)',
      '@media (min-width: 48rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@media (min-width: 48rem)': `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    },
    color: themeColors['--color-foreground'],
    '--tw-shadow': '0 1px 2px 0 var(--tw-shadow-color, #0000000d)',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
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
  attributes.className = cn(attributes.className, groupClass, inheritedClass)
  return <input type={type} data-slot="input" {...attributes} />
}

export { Input }
