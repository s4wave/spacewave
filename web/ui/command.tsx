import { cn } from '@s4wave/web/style/utils.js'
import {
  commandDialogMarker,
  commandGroupMarker,
  groupMarker,
} from '../style/relations.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  iconDefaults,
  iconRequired,
  commandIconMarker,
} from '../style/icon-defaults.stylex.js'
import * as React from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { LuSearch } from '@s4wave/web/images/icons.js'
import { type VariantProps } from 'class-variance-authority'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    maxHeight: 'inherit',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-popover'],
    color: themeColors['--color-popover-foreground'],
  },
  rule2: {
    top: 'auto',
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-translate-y': '0px',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: { default: null, '@media (min-width: 40rem)': 'none' },
  },
  rule3: { borderRadius: themeRadii['--radius-md'], backgroundColor: '#0000' },
  rule4: {
    position: 'relative',
    maxHeight: themeLayout['--max-height-dashboard-list'],
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-ui-outline'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-get-started']} 95%, transparent)`,
    '--tw-shadow':
      '0 20px 25px -5px var(--tw-shadow-color, #0000001a), 0 8px 10px -6px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule5: {
    position: 'relative',
    minHeight: `calc(${themeLayout['--spacing']} * 50)`,
    borderRadius: themeRadii['--radius-landing-launcher'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 11%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 0 0 1px var(--tw-shadow-color, oklch(0% 0 0 / .35)), 0 48px 96px -40px var(--tw-shadow-color, oklch(0% 0 0 / .8)), 0 14px 32px -18px var(--tw-shadow-color, oklch(0% 0 0 / .55))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule6: { borderRadius: themeRadii['--radius-md'] },
  rule7: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 60%, transparent)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    height: `calc(${themeLayout['--spacing']} * 12)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 4.5)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 4.5)`,
  },
  rule9: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    backgroundColor: '#0000',
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule10: {
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: 0,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule11: {
    height: `calc(${themeLayout['--spacing']} * 11)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-ui-outline'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule12: {
    fontSize: themeLayout['--text-landing-prompt'],
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground']} 70%, transparent)`,
    },
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule14: {
    minHeight: 0,
    flex: 1,
    scrollPaddingBlock: themeLayout['--spacing'],
    overflowX: 'hidden',
    overflowY: 'auto',
  },
  rule15: { paddingBottom: 0 },
  rule16: { backgroundColor: '#0000' },
  rule17: {
    backgroundColor: themeColors['--color-background-get-started'],
    paddingBottom: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule18: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule19: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule20: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule21: {
    overflow: 'hidden',
    padding: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
  },
  rule22: { paddingBlock: 0 },
  rule23: { paddingRight: 0, paddingLeft: 0 },
  rule24: { paddingBlock: themeLayout['--spacing'] },
  rule25: {
    marginInline: `calc(${themeLayout['--spacing']} * -1)`,
    height: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-border']} 60%, transparent)`,
  },
  rule26: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule27: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule28: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule29: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-warning'],
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
  },
  rule30: {
    position: 'relative',
    display: 'flex',
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
  },
  rule31: {
    position: 'relative',
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: 0.5, ':is([data-disabled="true"])': 0.5 },
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
    },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule32: {
    position: 'relative',
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
    cursor: 'default',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
    },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule33: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: '#0000',
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule34: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: '#0000',
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    color: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
  },
  rule35: {
    position: 'relative',
    marginInline: themeLayout['--spacing'],
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      ':is([data-selected="true"])': themeColors['--color-foreground'],
    },
    '--tw-outline-style': 'none',
    outlineStyle: {
      default: 'none',
      '@media (forced-colors: active)': 'solid',
    },
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outlineWidth: { default: null, '@media (forced-colors: active)': '2px' },
    outlineColor: { default: null, '@media (forced-colors: active)': '#0000' },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    pointerEvents: { default: null, ':is([data-disabled="true"])': 'none' },
    opacity: { default: null, ':is([data-disabled="true"])': 0.5 },
    backgroundColor: {
      default: null,
      ':is([data-selected="true"])': themeColors['--color-menu-selected'],
    },
  },
  rule36: {
    marginLeft: 'auto',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule37: {
    color: themeColors['--color-warning'],
    fontSize: 'inherit',
    lineHeight: 'inherit',
  },
  rule38: {
    fontSize: 'inherit',
    lineHeight: 'inherit',
    flexShrink: 0,
    paddingLeft: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
  },
  rule39: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 60%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: '10px',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const commandDialogStyles = stylex.create({
  palette: {
    top: 'auto',
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    maxHeight: themeLayout['--max-height-command-palette'],
    width: themeLayout['--width-command-editor'],
    '--tw-translate-y': '0px',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    overflow: 'hidden',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    padding: 0,
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'none',
    },
  },
  default: {
    top: '38%',
    maxHeight: 'min(28rem, 100vh - 6rem)',
    overflow: 'hidden',
    padding: 0,
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-xl)',
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule4: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 50%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '10px',
    lineHeight: 1,
    '--tw-leading': '1',
  },
})

const commandVariants = (props?: {
  variant?: 'default' | 'landing' | 'dashboard' | 'folder' | 'palette' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? stylexStyles2.rule6
      : variant0 === 'landing'
        ? stylexStyles2.rule5
        : variant0 === 'dashboard'
          ? stylexStyles2.rule4
          : variant0 === 'folder'
            ? stylexStyles2.rule3
            : variant0 === 'palette'
              ? stylexStyles2.rule2
              : false,
  ] as const
}

const commandInputWrapperVariants = (props?: {
  variant?: 'default' | 'landing' | 'dashboard' | 'folder' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? false
      : variant0 === 'landing'
        ? stylexStyles2.rule8
        : variant0 === 'dashboard'
          ? false
          : variant0 === 'folder'
            ? false
            : false,
  ] as const
}

const commandInputVariants = (props?: {
  variant?: 'default' | 'landing' | 'dashboard' | 'folder' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? stylexStyles2.rule13
      : variant0 === 'landing'
        ? stylexStyles2.rule12
        : variant0 === 'dashboard'
          ? stylexStyles2.rule11
          : variant0 === 'folder'
            ? stylexStyles2.rule10
            : false,
  ] as const
}

const commandListVariants = (props?: {
  variant?: 'default' | 'landing' | 'transparent' | 'palette' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? false
      : variant0 === 'landing'
        ? stylexStyles2.rule17
        : variant0 === 'transparent'
          ? stylexStyles2.rule16
          : variant0 === 'palette'
            ? stylexStyles2.rule15
            : false,
  ] as const
}

const commandEmptyVariants = (props?: {
  variant?: 'default' | 'dashboard' | 'folder' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? false
      : variant0 === 'dashboard'
        ? stylexStyles2.rule20
        : variant0 === 'folder'
          ? stylexStyles2.rule19
          : false,
  ] as const
}

// groupHeadingCaps sets group headings as small, spaced capitals. The
// landing launcher instead matches its prerendered sentence-case labels, so
// the headings do not change when the page hydrates.
const commandGroupVariants = (props?: {
  variant?: 'default' | 'compact' | 'palette' | 'landing' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? false
      : variant0 === 'compact'
        ? stylexStyles2.rule24
        : variant0 === 'palette'
          ? stylexStyles2.rule23
          : variant0 === 'landing'
            ? stylexStyles2.rule22
            : false,
  ] as const
}

const commandItemVariants = (props?: {
  variant?:
    | 'default'
    | 'landing'
    | 'dashboard'
    | 'dashboardIdentifier'
    | 'palette'
    | 'paletteDisabled'
    | 'back'
    | 'conflict'
    | 'compact'
    | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? stylexStyles2.rule27
      : variant0 === 'landing'
        ? stylexStyles2.rule35
        : variant0 === 'dashboard'
          ? [stylexStyles2.rule34, groupMarker]
          : variant0 === 'dashboardIdentifier'
            ? [stylexStyles2.rule33, groupMarker]
            : variant0 === 'palette'
              ? stylexStyles2.rule32
              : variant0 === 'paletteDisabled'
                ? stylexStyles2.rule31
                : variant0 === 'back'
                  ? stylexStyles2.rule30
                  : variant0 === 'conflict'
                    ? stylexStyles2.rule29
                    : variant0 === 'compact'
                      ? stylexStyles2.rule28
                      : stylexStyles2.rule27,
  ] as const
}

const commandShortcutVariants = (props?: {
  variant?: 'default' | 'brand' | 'warning' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? false
      : variant0 === 'brand'
        ? stylexStyles2.rule38
        : variant0 === 'warning'
          ? stylexStyles2.rule37
          : false,
  ] as const
}

const headingStyles = stylex.create({
  rule1: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 50%, transparent)`,
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
  },
})
const headingVariants = (props?: {
  variant?: 'default' | 'compact' | 'palette' | 'landing' | null
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? headingStyles.rule3
      : variant0 === 'compact'
        ? headingStyles.rule3
        : variant0 === 'palette'
          ? headingStyles.rule2
          : variant0 === 'landing'
            ? headingStyles.rule1
            : false,
  ] as const
}

const dialogPartStyles = stylex.create({
  wrapper: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '40px',
    },
  },
  input: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '40px',
    },
  },
  group: {
    paddingLeft: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '6px',
    },
    paddingRight: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '6px',
    },
    paddingTop: {
      default: null,
      [stylex.when.siblingBefore(':not([hidden])', commandGroupMarker)]: {
        default: null,
        [stylex.when.ancestor(':is(*)', commandDialogMarker)]: 0,
      },
    },
  },
  item: {
    paddingLeft: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '8px',
    },
    paddingRight: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: {
        default: '8px',
        '@media (pointer: fine)': '8px',
      },
    },
    paddingTop: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '6px',
    },
    paddingBottom: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '6px',
    },
  },
  plainHeading: {
    color: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]:
        themeColors['--color-foreground-alt'],
    },
    paddingInline: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]: '8px',
    },
    fontWeight: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]:
        themeFonts['--font-weight-medium'],
    },
    '--tw-font-weight': {
      default: null,
      [stylex.when.ancestor(':is(*)', commandDialogMarker)]:
        themeFonts['--font-weight-medium'],
    },
  },
  paletteGroup: {
    paddingInline: {
      default: null,
      ':is([data-variant="palette"])': '0px !important',
    },
  },
})
const dialogWrapperClass = stylex.props(dialogPartStyles.wrapper).className
const dialogInputClass = stylex.props(dialogPartStyles.input).className
const dialogGroupClass = stylex.props([
  dialogPartStyles.group,
  dialogPartStyles.paletteGroup,
]).className
const dialogItemClass = stylex.props(dialogPartStyles.item).className

/** Command renders the searchable command root with native caller styles. */
function Command({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive> &
    VariantProps<typeof commandVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <CommandPrimitive
      data-slot="command"
      {...stylexProps(
        [stylexStyles2.rule1, commandVariants({ variant: variant }), sx],
        props,
      )}
    />
  )
}

/** CommandDialog renders the command surface in a modal dialog. */
function CommandDialog({
  title = 'Command Palette',
  description = 'Search for a command to run...',
  children,
  sx,
  showCloseButton = true,
  variant,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string
  description?: string
  sx?: stylex.StyleXStyles
  className?: never
  showCloseButton?: boolean
  variant?: 'default' | 'palette'
}) {
  return (
    <Dialog {...props}>
      <DialogHeader sx={stylexStyles.rule1}>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <DialogContent
        sx={[
          variant === 'palette'
            ? commandDialogStyles.palette
            : commandDialogStyles.default,
          sx,
        ]}
        showCloseButton={showCloseButton}
      >
        <Command sx={commandDialogMarker}>{children}</Command>
      </DialogContent>
    </Dialog>
  )
}

/** CommandInput renders the search field and its owned search icon. */
function CommandInput({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.Input> &
    VariantProps<typeof commandInputVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const attributes = stylexProps(
    [stylexStyles2.rule9, commandInputVariants({ variant }), sx],
    props,
  )
  attributes.className = cn(attributes.className, dialogInputClass)
  return (
    <div
      data-slot="command-input-wrapper"
      className={cn(
        stylex.props([
          stylexStyles2.rule7,
          commandInputWrapperVariants({ variant }),
        ]).className,
        dialogWrapperClass,
      )}
    >
      <LuSearch sx={stylexStyles.rule2} />
      <CommandPrimitive.Input data-slot="command-input" {...attributes} />
    </div>
  )
}

/** CommandList styles the scrolling list and the library-owned size observer element. */
function CommandList({
  sx,
  sizerSx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.List> &
    VariantProps<typeof commandListVariants>,
  'className' | 'sizerProps'
> & { sx?: ComponentStyles; sizerSx?: ComponentStyles; className?: never }) {
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      sizerProps={stylexProps(sizerSx, {})}
      {...stylexProps(
        [stylexStyles2.rule14, commandListVariants({ variant: variant }), sx],
        props,
      )}
    />
  )
}

/** CommandEmpty renders the empty search state with its selected visual variant. */
function CommandEmpty({
  variant,
  sx,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.Empty> &
    VariantProps<typeof commandEmptyVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      {...stylexProps(
        [stylexStyles2.rule18, commandEmptyVariants({ variant: variant }), sx],
        props,
      )}
    />
  )
}

/** CommandGroup styles each group and its library-owned accessible heading. */
function CommandGroup({
  sx,
  headingSx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.Group> &
    VariantProps<typeof commandGroupVariants>,
  'className' | 'headingProps'
> & { sx?: ComponentStyles; headingSx?: ComponentStyles; className?: never }) {
  const attributes = stylexProps(
    [
      commandGroupMarker,
      stylexStyles2.rule21,
      commandGroupVariants({ variant }),
      sx,
    ],
    props,
  )
  attributes.className = cn(attributes.className, dialogGroupClass)

  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      data-variant={variant}
      headingProps={stylexProps(
        [
          headingVariants({ variant }),
          variant === null && dialogPartStyles.plainHeading,
          headingSx,
        ],
        {},
      )}
      {...attributes}
    />
  )
}

/** CommandSeparator renders a divider between visible command groups. */
function CommandSeparator({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.Separator> & {
    variant?: 'default' | 'subtle'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      {...stylexProps(
        [
          stylexStyles2.rule25,
          variant === 'subtle' && stylexStyles2.rule26,
          sx,
        ],
        props,
      )}
    />
  )
}

/** CommandItem renders one selectable command with scoped icon requirements. */
function CommandItem({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof CommandPrimitive.Item> &
    VariantProps<typeof commandItemVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const attributes = stylexProps(
    [commandIconMarker, commandItemVariants({ variant }), sx],
    props,
  )
  attributes.className = cn(attributes.className, dialogItemClass)
  return (
    <IconStylesProvider
      sx={[iconDefaults.command]}
      requiredSx={[iconRequired.command, iconRequired.commandDialog]}
    >
      <CommandPrimitive.Item data-slot="command-item" {...attributes} />
    </IconStylesProvider>
  )
}

/** CommandShortcut renders the optional keyboard shortcut alongside a command. */
function CommandShortcut({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<'span'> & VariantProps<typeof commandShortcutVariants>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <span
      data-slot="command-shortcut"
      {...stylexProps(
        [
          stylexStyles2.rule36,
          commandShortcutVariants({ variant: variant }),
          sx,
        ],
        props,
      )}
    />
  )
}

/** CommandFooter renders the keyboard navigation hints below the list. */
function CommandFooter({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="command-footer"
      {...stylexProps([stylexStyles2.rule39, sx], props)}
    >
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>&#8593;&#8595;</kbd>
        Navigate
      </span>
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>&#8629;</kbd>
        Select
      </span>
      <span sx={stylexStyles.rule3}>
        <kbd sx={stylexStyles.rule4}>Esc</kbd>
        Close
      </span>
    </div>
  )
}

export {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
  CommandFooter,
}
