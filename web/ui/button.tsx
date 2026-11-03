import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { buttonGroupControlStyles } from '../style/button-group.stylex.js'
import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'

import { Slot } from '@radix-ui/react-slot'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { buttonMarker, buttonIconStyles } from '../style/button-icons.stylex.js'
import {
  layout as themeLayout,
  radii as themeRadii,
  fonts as themeFonts,
  motion as themeMotion,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  base: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'inline-flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    pointerEvents: { default: null, ':disabled': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    whiteSpace: 'nowrap',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  unselected: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  category: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  unselectedFinder: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: '#0000',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  selectedFinder: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  errorGhost: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-error'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-error'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  destructiveSubtle: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  debugGhost: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  debugOutline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  selectedRow: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  unselectedRow: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  capture: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([data-capturing="true"])': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
      ':is([data-capturing="true"])':
        'var(--tw-ring-inset, ) 0 0 0 calc(4px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':is([data-capturing="true"])': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
      ':is([data-capturing="true"])': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':is([data-capturing="true"])':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      ':is([data-capturing="true"])': themeColors['--color-brand'],
    },
    fontWeight: 'var(--font-weight-normal)',
  },
  selectedBrand: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  brandSoft: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 35%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  muted: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  brandGhost: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-brand'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  dangerGhost: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  destructiveOutline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-destructive'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  quiet: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  brandOutline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  link: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-primary'],
    fontWeight: themeFonts['--font-weight-medium'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    textUnderlineOffset: '4px',
  },
  ghost: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-accent'],
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-accent-foreground'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  secondary: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-secondary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-secondary']} 80%, transparent)`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-secondary-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  outline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderColor: themeColors['--color-input'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: themeColors['--color-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-accent'],
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-accent-foreground'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  destructive: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 90%, transparent)`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-destructive-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  default: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-primary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-primary']} 90%, transparent)`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-primary-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
})

const sizeStyles = stylex.create({
  default: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  sm: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  lg: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  icon: {
    height: `calc(${themeLayout['--spacing']} * 9)`,
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  xs: {
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
  },
  toolbar: {
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  toolbarSm: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  toolbarWide: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  brandWide: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  toolbarText: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  compact8: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  iconSm: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  iconXs: {
    padding: 0,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  row: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    height: 'auto',
  },
  finderRow: {
    borderRadius: themeRadii['--radius-lg'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    height: 'auto',
  },
  commandRow: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: 'auto',
  },
  category: {
    borderRadius: 0,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: 'auto',
  },
})

const variantStyles = {
  default: styles['default'],
  destructive: styles['destructive'],
  outline: styles['outline'],
  secondary: styles['secondary'],
  ghost: styles['ghost'],
  link: styles['link'],
  brandOutline: styles['brandOutline'],
  quiet: styles['quiet'],
  destructiveOutline: styles['destructiveOutline'],
  dangerGhost: styles['dangerGhost'],
  brandGhost: styles['brandGhost'],
  muted: styles['muted'],
  brandSoft: styles['brandSoft'],
  selectedBrand: styles['selectedBrand'],
  capture: styles['capture'],
  unselectedRow: styles['unselectedRow'],
  selectedRow: styles['selectedRow'],
  debugOutline: styles['debugOutline'],
  debugGhost: styles['debugGhost'],
  destructiveSubtle: styles['destructiveSubtle'],
  errorGhost: styles['errorGhost'],
  selectedFinder: styles['selectedFinder'],
  unselectedFinder: styles['unselectedFinder'],
  category: styles['category'],
}

const groupClass = stylex.props(buttonGroupControlStyles.child).className

/** ButtonProps configures the button's visual variant and native behavior. */
export interface ButtonProps extends Omit<
  React.ComponentPropsWithRef<'button'>,
  'className'
> {
  className?: never
  sx?: ComponentStyles
  variant?: keyof typeof variantStyles | null
  size?: keyof typeof sizeStyles | null
  asChild?: boolean
}

/** buttonVariants returns compiled styles for sx or stylex.props composition. */
export function buttonVariants({
  variant,
  size,
  sx,
}: Pick<ButtonProps, 'variant' | 'size' | 'sx'> = {}) {
  return [
    styles.base,
    variant === undefined
      ? variantStyles.default
      : variant
        ? variantStyles[variant]
        : styles.unselected,
    size === undefined ? sizeStyles.default : size && sizeStyles[size],
    sx,
  ] as const
}

/** Button renders an action or lends its behavior and styles to one child.
 * Shared icon exports inherit its SVG dimensions and pointer rules.
 */
function Button({ sx, variant, size, asChild = false, ...props }: ButtonProps) {
  // Retain dynamic StyleX variables alongside the native inline override.
  const attributes = stylexProps(
    [buttonVariants({ variant, size, sx }), buttonMarker],
    props,
  )

  // Keep DOM-gated control rules independent of the button's overridable defaults.
  const relationshipClass = useControlStyles()
  attributes.className = [attributes.className, groupClass, relationshipClass]
    .filter(Boolean)
    .join(' ')
  const Comp = asChild ? Slot : 'button'
  return (
    <IconStylesProvider requiredSx={buttonIconStyles.icon}>
      <Comp {...attributes} />
    </IconStylesProvider>
  )
}
Button.displayName = 'Button'
export { Button }
