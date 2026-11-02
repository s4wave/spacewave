import { useCallback, useState, type ReactNode } from 'react'
import {
  LuActivity,
  LuArrowLeft,
  LuBriefcase,
  LuCheck,
  LuCpu,
  LuListTodo,
  LuPlay,
  LuRotateCw,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

import { groupMarker } from '../style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule9: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule12: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule15: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule17: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule18: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-white'],
  },
  rule19: { backgroundColor: themeColors['--color-green-600'] },
  rule20: { backgroundColor: themeColors['--color-blue-600'] },
  rule21: { backgroundColor: themeColors['--color-red-600'] },
  rule22: { backgroundColor: themeColors['--color-neutral-600'] },
  rule23: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule24: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: { minWidth: 0 },
  rule26: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule27: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule28: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule29: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule30: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule31: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule32: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule33: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule34: {
    display: { default: 'none', '@media (min-width: 40rem)': 'block' },
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule35: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule36: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule37: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule38: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule39: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule40: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule41: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule42: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule43: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule44: {
    width: `calc(${themeLayout['--spacing']} * 4.5)`,
    height: `calc(${themeLayout['--spacing']} * 4.5)`,
    color: themeColors['--color-brand'],
  },
  rule45: { minWidth: 0, flex: 1 },
  rule46: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule47: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule48: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule49: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule50: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule51: {
    marginInline: `calc(${themeLayout['--spacing']} * -3)`,
    marginTop: `calc(${themeLayout['--spacing']} * -3)`,
    marginBottom: 0,
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    alignItems: 'flex-end',
    gap: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule52: {
    position: 'relative',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingTop: themeLayout['--spacing'],
    paddingBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule53: { color: themeColors['--color-foreground'] },
  rule54: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 70%, transparent)`,
      },
    },
  },
  rule55: {
    position: 'absolute',
    right: themeLayout['--spacing'],
    bottom: 0,
    left: themeLayout['--spacing'],
    height: `calc(${themeLayout['--spacing']} * .5)`,
    borderTopLeftRadius: '.25rem',
    borderTopRightRadius: '.25rem',
    backgroundColor: themeColors['--color-foreground'],
  },
  rule56: {
    marginInline: `calc(${themeLayout['--spacing']} * -3)`,
    marginTop: `calc(${themeLayout['--spacing']} * -3)`,
    marginBottom: 0,
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'flex-end',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule57: {
    position: 'relative',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule58: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
      },
    },
  },
  rule59: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 2)`,
    bottom: 0,
    left: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * .5)`,
    borderTopLeftRadius: '.25rem',
    borderTopRightRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule60: {
    display: 'inline-flex',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    padding: themeLayout['--spacing'],
  },
  rule61: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule62: {
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule63: {
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: '#0000',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 90%, transparent)`,
      },
    },
  },
  rule64: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule65: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule66: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule67: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule68: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule69: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'flex-end',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule70: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule71: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule72: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule73: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
  },
  rule74: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 10%, transparent)`,
  },
  rule75: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
  rule76: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-blue-400']} 10%, transparent)`,
  },
  rule77: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule78: { color: themeColors['--color-emerald-300'] },
  rule79: { color: themeColors['--color-destructive'] },
  rule80: { color: themeColors['--color-blue-300'] },
  rule81: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule82: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule83: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule84: {
    color: themeColors['--color-destructive'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule85: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

// ForgeViewerDebug renders prototype variants for the Forge ObjectViewer
// surfaces (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each
// section shows the current production look first, then candidate variants
// aligned with guides/alpha-ui-design-system.org.
export function ForgeViewerDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <button
          type="button"
          onClick={goBack}
          sx={stylexStyles.rule3}
          aria-label="Back"
        >
          <LuArrowLeft sx={stylexStyles.rule4} />
        </button>
        <span sx={stylexStyles.rule5}>Forge Viewer UI Prototype</span>
      </div>

      <div sx={stylexStyles.rule6}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-6 */ 'mx-auto max-w-3xl space-y-6'
          }
        >
          <Intro />
          <StateBadgeSection />
          <EntityRowSection />
          <StatTileSection />
          <EmptyStateSection />
          <TabBarSection />
          <ViewerShellSection />
        </div>
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Intro
// -----------------------------------------------------------------------

function Intro() {
  return (
    <div sx={stylexStyles.rule7}>
      <p sx={stylexStyles.rule8}>Forge ObjectViewer variant gallery</p>
      <p sx={stylexStyles.rule9}>
        Side-by-side variants for the primitives that compose every Forge viewer
        (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each section
        shows the historical production rendering, then candidate variants drawn
        from the modern token set in guides/alpha-ui-design-system.org.
      </p>
      <p sx={stylexStyles.rule10}>Decisions live in production code:</p>
      <ul
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-0.5 */ 'text-foreground-alt/70 mt-1 list-disc space-y-0.5 pl-5 text-xs leading-relaxed'
        }
      >
        <li>
          State badge: B (tinted pill) is default, C (dot + label) is the list
          variant via prop.
        </li>
        <li>Entity rows: C (compact dense single-line) for ForgeEntityList.</li>
        <li>
          Stat tile: one figure strip (FigureStrip, web/ui/FigureStrip.tsx).
        </li>
        <li>Empty state: B (compact single-line, foreground-alt/40).</li>
        <li>Tab bar: C (pill tabs) in ForgeViewerShell.</li>
      </ul>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section wrapper
// -----------------------------------------------------------------------

interface SectionProps {
  title: string
  description: string
  children: ReactNode
}

function Section({ title, description, children }: SectionProps) {
  return (
    <section
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div>
        <h2 sx={stylexStyles.rule5}>{title}</h2>
        <p sx={stylexStyles.rule11}>{description}</p>
      </div>
      {children}
    </section>
  )
}

interface VariantProps {
  label: string
  note?: string
  children: ReactNode
}

function Variant({ label, note, children }: VariantProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'border-foreground/6 bg-background-card/30 space-y-2 rounded-lg border p-3.5'
      }
    >
      <div sx={stylexStyles.rule12}>
        <span sx={stylexStyles.rule13}>{label}</span>
        {note && <span sx={stylexStyles.rule14}>{note}</span>}
      </div>
      <div sx={stylexStyles.rule15}>{children}</div>
    </div>
  )
}

// -----------------------------------------------------------------------
// State badges
// -----------------------------------------------------------------------

type StateTone = 'idle' | 'active' | 'success' | 'warning' | 'error'

interface StateSpec {
  label: string
  tone: StateTone
}

const STATES: StateSpec[] = [
  { label: 'PENDING', tone: 'idle' },
  { label: 'RUNNING', tone: 'active' },
  { label: 'CHECKING', tone: 'active' },
  { label: 'COMPLETE', tone: 'success' },
  { label: 'RETRY', tone: 'error' },
]

function StateBadgeSection() {
  return (
    <Section
      title="State badges"
      description="Forge enums (TaskState, PassState, ExecutionState, JobState) all render through StateBadge. Heavy solid colors fight the rest of the UI; opacity-tinted pills match billing/session badges."
    >
      <Variant
        label="A. Current (web/forge/StateBadge.tsx)"
        note="bg-blue-600 / bg-green-600 / bg-red-600 + white text"
      >
        <div sx={stylexStyles.rule16}>
          {STATES.map((s) => (
            <CurrentBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Tinted opacity pill"
        note="border-{tone}/15 bg-{tone}/5 text-{tone}-tone, matches billing status pills"
      >
        <div sx={stylexStyles.rule16}>
          {STATES.map((s) => (
            <TintedBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Dot + label minimal"
        note="text-foreground-alt/70 with semantic dot, lowest visual weight"
      >
        <div sx={stylexStyles.rule17}>
          {STATES.map((s) => (
            <DotBadge key={s.label} {...s} />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentBadge({ label }: StateSpec) {
  return (
    <span
      sx={[
        stylexStyles.rule18,
        label === 'COMPLETE'
          ? stylexStyles.rule19
          : label === 'RUNNING' || label === 'CHECKING'
            ? stylexStyles.rule20
            : label === 'RETRY'
              ? stylexStyles.rule21
              : stylexStyles.rule22,
      ]}
    >
      {label}
    </span>
  )
}

const TONE_TINT: Record<StateTone, string> = {
  idle: 'border-foreground/15 bg-foreground/5 text-foreground-alt/70',
  active: 'border-blue-400/20 bg-blue-400/8 text-blue-300',
  success: 'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  warning: 'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error: 'border-destructive/30 bg-destructive/8 text-destructive',
}

function TintedBadge({ label, tone }: StateSpec) {
  return (
    <span
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'inline-flex items-center rounded-full border px-2 py-0.5 micro-text font-medium tracking-widest uppercase select-none',
          TONE_TINT[tone],
        )
      }
    >
      {label}
    </span>
  )
}

const TONE_DOT: Record<StateTone, string> = {
  idle: 'bg-foreground/30',
  active: 'bg-blue-400',
  success: 'bg-emerald-400',
  warning: 'bg-amber-400',
  error: 'bg-destructive',
}

function DotBadge({ label, tone }: StateSpec) {
  return (
    <span sx={stylexStyles.rule23}>
      <span
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            'size-1.5 rounded-full',
            TONE_DOT[tone],
          )
        }
      />
      {label}
    </span>
  )
}

// -----------------------------------------------------------------------
// Entity row cards
// -----------------------------------------------------------------------

interface RowSample {
  name: string
  meta: string
  state: StateSpec
}

const ROW_SAMPLES: RowSample[] = [
  {
    name: 'cluster/forge/build',
    meta: '3 workers, peer 12D3KooW...x9q',
    state: { label: 'RUNNING', tone: 'active' },
  },
  {
    name: 'task/clone-repository',
    meta: 'Pass #4, started 22:14',
    state: { label: 'CHECKING', tone: 'active' },
  },
  {
    name: 'task/build-image',
    meta: '2 outputs',
    state: { label: 'COMPLETE', tone: 'success' },
  },
  {
    name: 'task/upload-artifact',
    meta: 'fail: signature mismatch',
    state: { label: 'RETRY', tone: 'error' },
  },
]

function EntityRowSection() {
  return (
    <Section
      title="Entity rows (linked entity lists)"
      description="ForgeEntityList renders Task, Pass, Execution, Worker, Job rows. Current rows use rounded-px-3 with no hover; design-system cards use rounded-lg with /6 to /12 hover."
    >
      <Variant
        label="A. Current (border-foreground/6 bg-background-card/20 rounded px-3 py-2)"
        note="No hover state, tight radius"
      >
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
          }
        >
          {ROW_SAMPLES.map((s) => (
            <CurrentRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Glass card with hover (rounded-lg, /30 -> /50)"
        note="Matches InfoCard radius, design-system hover transitions"
      >
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
          }
        >
          {ROW_SAMPLES.map((s) => (
            <GlassRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Compact dense single-line"
        note="One line, leading icon, brand divider, badge tail"
      >
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'border-foreground/6 bg-background-card/30 divide-foreground/6 divide-y rounded-lg border'
          }
        >
          {ROW_SAMPLES.map((s) => (
            <DenseRow key={s.name} sample={s} />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentRow({ sample }: { sample: RowSample }) {
  return (
    <div sx={stylexStyles.rule24}>
      <div sx={stylexStyles.rule25}>
        <div sx={stylexStyles.rule26}>{sample.name}</div>
        <div sx={stylexStyles.rule27}>{sample.meta}</div>
      </div>
      <CurrentBadge {...sample.state} />
    </div>
  )
}

function GlassRow({ sample }: { sample: RowSample }) {
  return (
    <div sx={[stylexStyles2.rule1, groupMarker]}>
      <div sx={stylexStyles.rule28}>
        <span
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:bg-foreground/8 */ 'bg-foreground/5 group-hover:bg-foreground/8 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors'
          }
        >
          <LuListTodo sx={stylexStyles.rule29} />
        </span>
        <div sx={stylexStyles.rule25}>
          <div sx={stylexStyles.rule30}>{sample.name}</div>
          <div sx={stylexStyles.rule27}>{sample.meta}</div>
        </div>
      </div>
      <TintedBadge {...sample.state} />
    </div>
  )
}

function DenseRow({ sample }: { sample: RowSample }) {
  return (
    <div sx={stylexStyles.rule31}>
      <LuListTodo sx={stylexStyles.rule32} />
      <span sx={stylexStyles.rule33}>{sample.name}</span>
      <span sx={stylexStyles.rule34}>{sample.meta}</span>
      <DotBadge {...sample.state} />
    </div>
  )
}

// -----------------------------------------------------------------------
// Stat tiles
// -----------------------------------------------------------------------

function StatTileSection() {
  return (
    <Section
      title="Stat tiles (Job progress, Task counts, Capacity)"
      description="Current viewers stack large numeric tiles using InfoCard. Compact stat groups (billing) and StatCard (session dashboard) are lighter."
    >
      <Variant
        label="A. Current (InfoCard text-2xl)"
        note="As used in ForgeJobViewer / ForgeTaskViewer overview"
      >
        <div sx={stylexStyles.rule35}>
          <CurrentStatCard
            icon={<LuListTodo sx={stylexStyles.rule36} />}
            title="Tasks"
            value="6/12"
            detail="50% complete"
          />
          <CurrentStatCard
            icon={<LuPlay sx={stylexStyles.rule36} />}
            title="Passes"
            value="4"
            detail="Current nonce 4"
          />
        </div>
      </Variant>

      <Variant
        label="B. Compact stat group (billing-style)"
        note="Uppercase label, no card per metric"
      >
        <div sx={stylexStyles.rule37}>
          <CompactStat label="Tasks" value="6/12" detail="50% complete" />
          <CompactStat label="Passes" value="4" detail="Current nonce 4" />
        </div>
      </Variant>

      <Variant
        label="C. Inline StatCard (session dashboard)"
        note="Brand icon box, tiny label, single-line value"
      >
        <div sx={stylexStyles.rule35}>
          <InlineStat
            icon={LuListTodo}
            label="Tasks complete"
            value="6 of 12"
          />
          <InlineStat icon={LuPlay} label="Passes" value="4 (nonce 4)" />
        </div>
      </Variant>
    </Section>
  )
}

function CurrentStatCard({
  icon,
  title,
  value,
  detail,
}: {
  icon: ReactNode
  title: string
  value: string
  detail: string
}) {
  return (
    <div sx={stylexStyles.rule7}>
      <h3 sx={stylexStyles.rule38}>
        <span sx={stylexStyles.rule39}>{icon}</span>
        {title}
      </h3>
      <div sx={stylexStyles.rule40}>{value}</div>
      <div sx={stylexStyles.rule41}>{detail}</div>
    </div>
  )
}

function CompactStat({
  label,
  value,
  detail,
}: {
  label: string
  value: string
  detail: string
}) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1 */ 'space-y-1'
      }
    >
      <div sx={stylexStyles.rule42}>{label}</div>
      <div sx={stylexStyles.rule43}>{value}</div>
      <div sx={stylexStyles.rule14}>{detail}</div>
    </div>
  )
}

function InlineStat({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof LuListTodo
  label: string
  value: string
}) {
  return (
    <div sx={[stylexStyles2.rule2, groupMarker]}>
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:bg-brand/15 */ 'bg-brand/10 group-hover:bg-brand/15 flex size-9 shrink-0 items-center justify-center rounded transition duration-150'
        }
      >
        <Icon sx={stylexStyles.rule44} />
      </div>
      <div sx={stylexStyles.rule45}>
        <p sx={stylexStyles.rule46}>{label}</p>
        <p sx={stylexStyles.rule47}>{value}</p>
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Empty states
// -----------------------------------------------------------------------

function EmptyStateSection() {
  return (
    <Section
      title="Empty states"
      description="Forge viewers use centered hero blocks ('No tasks in job', py-4 text-center). Design system prefers compact single-line states with muted icon + text."
    >
      <Variant
        label="A. Current (text-muted-foreground py-4 text-center)"
        note="Centered, no icon, deprecated text-muted-foreground token"
      >
        <div sx={stylexStyles.rule48}>No tasks in job</div>
      </Variant>

      <Variant
        label="B. Compact single-line (design-system)"
        note="Muted icon + text, text-foreground-alt/40"
      >
        <div sx={stylexStyles.rule7}>
          <div sx={stylexStyles.rule49}>
            <LuListTodo sx={stylexStyles.rule50} />
            <span>No tasks linked to this job yet</span>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Tab bar
// -----------------------------------------------------------------------

const TABS = ['Overview', 'Tasks', 'Pass History', 'Outputs']

function TabBarSection() {
  const [activeA, setActiveA] = useState(0)
  const [activeB, setActiveB] = useState(0)
  const [activeC, setActiveC] = useState(0)

  return (
    <Section
      title="Tab bar (ForgeViewerShell)"
      description="Current bar uses bg-foreground underline. Brand-tinted underline ties it to the rest of the UI; pill tabs read better when there are 3+ short labels."
    >
      <Variant
        label="A. Current (foreground underline, h-8 row)"
        note="From web/forge/ForgeViewerShell.tsx"
      >
        <CurrentTabs active={activeA} onSelect={setActiveA} />
      </Variant>

      <Variant
        label="B. Brand underline + softer inactive"
        note="bg-brand/80 underline, text-foreground-alt/50 -> /80 inactive"
      >
        <BrandTabs active={activeB} onSelect={setActiveB} />
      </Variant>

      <Variant
        label="C. Pill tabs"
        note="bg-foreground/5 pill row with bg-brand/10 selected"
      >
        <PillTabs active={activeC} onSelect={setActiveC} />
      </Variant>
    </Section>
  )
}

function CurrentTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div sx={stylexStyles.rule51}>
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          sx={[
            stylexStyles.rule52,
            i === active ? stylexStyles.rule53 : stylexStyles.rule54,
          ]}
        >
          {label}
          {i === active && <span sx={stylexStyles.rule55} />}
        </button>
      ))}
    </div>
  )
}

function BrandTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div sx={stylexStyles.rule56}>
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          sx={[
            stylexStyles.rule57,
            i === active ? stylexStyles.rule53 : stylexStyles.rule58,
          ]}
        >
          {label}
          {i === active && <span sx={stylexStyles.rule59} />}
        </button>
      ))}
    </div>
  )
}

function PillTabs({
  active,
  onSelect,
}: {
  active: number
  onSelect: (i: number) => void
}) {
  return (
    <div sx={stylexStyles.rule60}>
      {TABS.map((label, i) => (
        <button
          key={label}
          type="button"
          onClick={() => onSelect(i)}
          sx={[
            stylexStyles.rule61,
            i === active ? stylexStyles.rule62 : stylexStyles.rule63,
          ]}
        >
          {label}
        </button>
      ))}
    </div>
  )
}

// -----------------------------------------------------------------------
// Combined viewer shell
// -----------------------------------------------------------------------

function ViewerShellSection() {
  const [tab, setTab] = useState(0)
  return (
    <Section
      title="Combined viewer mock"
      description="Full ForgeViewerShell layout using the candidate variants: brand underline tabs, tinted state badge, glass entity rows, compact empty state. Compare against ForgeJobViewer in repos/spacewave/app/forge/."
    >
      <div sx={stylexStyles.rule64}>
        <div sx={stylexStyles.rule65}>
          <div sx={stylexStyles.rule66}>
            <LuBriefcase sx={stylexStyles.rule4} />
            <span sx={stylexStyles.rule67}>Build pipeline</span>
            <TintedBadge label="RUNNING" tone="active" />
          </div>
          <div sx={stylexStyles.rule68}>Job</div>
        </div>

        <div sx={stylexStyles.rule69}>
          {TABS.slice(0, 3).map((label, i) => (
            <button
              key={label}
              type="button"
              onClick={() => setTab(i)}
              sx={[
                stylexStyles.rule57,
                i === tab ? stylexStyles.rule53 : stylexStyles.rule58,
              ]}
            >
              {label}
              {i === tab && <span sx={stylexStyles.rule59} />}
            </button>
          ))}
        </div>

        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3 px-4 py-3'
          }
        >
          {tab === 0 && (
            <>
              <div sx={stylexStyles.rule35}>
                <InlineStat
                  icon={LuListTodo}
                  label="Tasks complete"
                  value="6 of 12"
                />
                <InlineStat icon={LuPlay} label="Passes" value="4" />
              </div>
              <div sx={stylexStyles.rule70}>
                <MiniStat
                  icon={LuActivity}
                  label="Active"
                  value="3"
                  tone="active"
                />
                <MiniStat
                  icon={LuCheck}
                  label="Done"
                  value="6"
                  tone="success"
                />
                <MiniStat
                  icon={LuRotateCw}
                  label="Retry"
                  value="1"
                  tone="error"
                />
              </div>
            </>
          )}
          {tab === 1 && (
            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
              }
            >
              {ROW_SAMPLES.map((s) => (
                <GlassRow key={s.name} sample={s} />
              ))}
            </div>
          )}
          {tab === 2 && (
            <div sx={stylexStyles.rule7}>
              <div sx={stylexStyles.rule49}>
                <LuPlay sx={stylexStyles.rule50} />
                <span>No passes recorded yet</span>
              </div>
            </div>
          )}
        </div>

        <div sx={stylexStyles.rule71}>
          <ShellAction icon={<LuCpu sx={stylexStyles.rule36} />} label="Run" />
          <ShellAction
            icon={<LuTriangleAlert sx={stylexStyles.rule36} />}
            label="Cancel"
            variant="destructive"
          />
        </div>
      </div>
    </Section>
  )
}

function MiniStat({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: typeof LuActivity
  label: string
  value: string
  tone: StateTone
}) {
  return (
    <div sx={stylexStyles.rule72}>
      <span
        sx={[
          stylexStyles.rule73,
          tone === 'success'
            ? stylexStyles.rule74
            : tone === 'error'
              ? stylexStyles.rule75
              : stylexStyles.rule76,
        ]}
      >
        <Icon
          sx={[
            stylexStyles.rule77,
            tone === 'success'
              ? stylexStyles.rule78
              : tone === 'error'
                ? stylexStyles.rule79
                : stylexStyles.rule80,
          ]}
        />
      </span>
      <div sx={stylexStyles.rule25}>
        <div sx={stylexStyles.rule81}>{label}</div>
        <div sx={stylexStyles.rule82}>{value}</div>
      </div>
    </div>
  )
}

function ShellAction({
  icon,
  label,
  variant,
}: {
  icon: ReactNode
  label: string
  variant?: 'default' | 'destructive'
}) {
  return (
    <button
      type="button"
      sx={[
        stylexStyles.rule83,
        variant === 'destructive' ? stylexStyles.rule84 : stylexStyles.rule85,
      ]}
    >
      {icon}
      {label}
    </button>
  )
}
