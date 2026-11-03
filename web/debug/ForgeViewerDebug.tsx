import { rowDividers } from '@s4wave/web/style/row-dividers.stylex.js'

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

import { groupMarker as groupMarker2 } from '../style/relations.stylex.js'

const stylexStyles7 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
})

const stylexStyles6 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'inline-flex',
    fontSize: '.6rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-blue-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-blue-400']} 8%, transparent)`,
    color: themeColors['--color-blue-300'],
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 8%, transparent)`,
    color: themeColors['--color-emerald-300'],
  },
  rule4: {
    borderColor: 'color-mix(in oklab, var(--color-amber-400) 20%, transparent)',
    backgroundColor:
      'color-mix(in oklab, var(--color-amber-400) 8%, transparent)',
    color: 'var(--color-amber-300)',
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
  },
  rule7: { backgroundColor: themeColors['--color-blue-400'] },
  rule8: { backgroundColor: themeColors['--color-emerald-400'] },
  rule9: { backgroundColor: 'var(--color-amber-400)' },
  rule10: { backgroundColor: themeColors['--color-destructive'] },
})

const stylexStyles4 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-3xl)' },
  rule2: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    listStyleType: 'disc',
    marginTop: themeLayout['--spacing'],
    paddingLeft: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule4: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const childSpacing = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
  },
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
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

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule2: {
    '--tw-duration': '.15s',
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    justifyContent: 'space-between',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-duration': '.15s',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
      },
    },
    display: 'flex',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule10: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'baseline',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule14: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
  },
  rule16: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule17: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule18: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-white'],
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: { backgroundColor: themeColors['--color-green-600'] },
  rule20: { backgroundColor: themeColors['--color-blue-600'] },
  rule21: { backgroundColor: themeColors['--color-red-600'] },
  rule22: { backgroundColor: themeColors['--color-neutral-600'] },
  rule23: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'inline-flex',
    fontSize: '.6rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule24: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule25: { minWidth: 0 },
  rule26: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule27: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule28: {
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule29: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule30: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule31: {
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule32: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule33: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule34: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: { default: 'none', '@media (min-width: 40rem)': 'block' },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule35: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
  },
  rule36: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule37: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
  },
  rule38: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule39: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule40: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
  },
  rule41: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule42: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule43: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
  },
  rule44: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4.5)`,
    width: `calc(${themeLayout['--spacing']} * 4.5)`,
  },
  rule45: { flex: 1, minWidth: 0 },
  rule46: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule47: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule48: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule49: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule50: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule51: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: 0,
    marginInline: `calc(${themeLayout['--spacing']} * -3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-end',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    marginBottom: 0,
    marginTop: `calc(${themeLayout['--spacing']} * -3)`,
  },
  rule52: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    position: 'relative',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingTop: themeLayout['--spacing'],
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
    backgroundColor: themeColors['--color-foreground'],
    position: 'absolute',
    borderTopLeftRadius: '.25rem',
    borderTopRightRadius: '.25rem',
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * .5)`,
    left: themeLayout['--spacing'],
    right: themeLayout['--spacing'],
  },
  rule56: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: themeLayout['--spacing'],
    marginInline: `calc(${themeLayout['--spacing']} * -3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-end',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    marginBottom: 0,
    marginTop: `calc(${themeLayout['--spacing']} * -3)`,
  },
  rule57: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    position: 'relative',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 1.5)`,
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
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
    position: 'absolute',
    borderTopLeftRadius: '.25rem',
    borderTopRightRadius: '.25rem',
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * .5)`,
    left: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule60: {
    padding: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'inline-flex',
  },
  rule61: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule62: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule63: {
    borderColor: '#0000',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 90%, transparent)`,
      },
    },
  },
  rule64: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule65: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule66: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule67: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule68: {
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule69: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-end',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule70: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
  },
  rule71: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule72: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    display: 'flex',
  },
  rule73: {
    borderRadius: '.25rem',
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
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
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule78: { color: themeColors['--color-emerald-300'] },
  rule79: { color: themeColors['--color-destructive'] },
  rule80: { color: themeColors['--color-blue-300'] },
  rule81: {
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.55rem',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule82: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule83: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule84: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
  },
  rule85: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
  },
})

// ForgeViewerDebug renders prototype variants for the Forge ObjectViewer
// surfaces (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each
// section shows the current production look first, then candidate variants
// aligned with guides/alpha-ui-design-system.org.
export function ForgeViewerDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={[stylexStyles.rule1, sx]}>
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
        <div sx={stylexStyles4.rule1}>
          <Intro sx={childSpacing.s6} />
          <StateBadgeSection sx={childSpacing.s6} />
          <EntityRowSection sx={childSpacing.s6} />
          <StatTileSection sx={childSpacing.s6} />
          <EmptyStateSection sx={childSpacing.s6} />
          <TabBarSection sx={childSpacing.s6} />
          <ViewerShellSection sx={childSpacing.s6} />
        </div>
      </div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Intro
// -----------------------------------------------------------------------

function Intro({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <div sx={[stylexStyles.rule7, sx]}>
      <p sx={stylexStyles.rule8}>Forge ObjectViewer variant gallery</p>
      <p sx={stylexStyles.rule9}>
        Side-by-side variants for the primitives that compose every Forge viewer
        (Task, Job, Pass, Worker, Cluster, Execution, Dashboard). Each section
        shows the historical production rendering, then candidate variants drawn
        from the modern token set in guides/alpha-ui-design-system.org.
      </p>
      <p sx={stylexStyles.rule10}>Decisions live in production code:</p>
      <ul sx={stylexStyles4.rule2}>
        <li sx={childSpacing.s0_5}>
          State badge: B (tinted pill) is default, C (dot + label) is the list
          variant via prop.
        </li>
        <li sx={childSpacing.s0_5}>
          Entity rows: C (compact dense single-line) for ForgeEntityList.
        </li>
        <li sx={childSpacing.s0_5}>
          Stat tile: one figure strip (FigureStrip, web/ui/FigureStrip.tsx).
        </li>
        <li sx={childSpacing.s0_5}>
          Empty state: B (compact single-line, foreground-alt/40).
        </li>
        <li sx={childSpacing.s0_5}>
          Tab bar: C (pill tabs) in ForgeViewerShell.
        </li>
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
  sx?: stylex.StyleXStyles
}

function Section({ title, description, children, sx }: SectionProps) {
  return (
    <section sx={sx}>
      <div sx={childSpacing.s3}>
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
  sx?: stylex.StyleXStyles
}

function Variant({ label, note, children, sx }: VariantProps) {
  return (
    <div sx={[stylexStyles4.rule3, sx]}>
      <div sx={[stylexStyles.rule12, childSpacing.s2]}>
        <span sx={stylexStyles.rule13}>{label}</span>
        {note && <span sx={stylexStyles.rule14}>{note}</span>}
      </div>
      <div sx={[stylexStyles.rule15, childSpacing.s2]}>{children}</div>
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

function StateBadgeSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
      title="State badges"
      description="Forge enums (TaskState, PassState, ExecutionState, JobState) all render through StateBadge. Heavy solid colors fight the rest of the UI; opacity-tinted pills match billing/session badges."
    >
      <Variant
        sx={childSpacing.s3}
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
        sx={childSpacing.s3}
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
        sx={childSpacing.s3}
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

const TONE_TINT: Record<StateTone, stylex.StyleXStyles> = {
  idle: [stylexStyles5.rule1],
  active: [stylexStyles5.rule2],
  success: [stylexStyles5.rule3],
  warning: [stylexStyles5.rule4],
  error: [stylexStyles5.rule5],
}

function TintedBadge({ label, tone }: StateSpec) {
  return <span sx={[[stylexStyles6.rule1], [TONE_TINT[tone]]]}>{label}</span>
}

const TONE_DOT: Record<StateTone, stylex.StyleXStyles> = {
  idle: [stylexStyles5.rule6],
  active: [stylexStyles5.rule7],
  success: [stylexStyles5.rule8],
  warning: [stylexStyles5.rule9],
  error: [stylexStyles5.rule10],
}

function DotBadge({ label, tone }: StateSpec) {
  return (
    <span sx={stylexStyles.rule23}>
      <span sx={[[stylexStyles6.rule2], [TONE_DOT[tone]]]} />
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

function EntityRowSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
      title="Entity rows (linked entity lists)"
      description="ForgeEntityList renders Task, Pass, Execution, Worker, Job rows. Current rows use rounded-px-3 with no hover; design-system cards use rounded-lg with /6 to /12 hover."
    >
      <Variant
        sx={childSpacing.s3}
        label="A. Current (border-foreground/6 bg-background-card/20 rounded px-3 py-2)"
        note="No hover state, tight radius"
      >
        <div>
          {ROW_SAMPLES.map((s) => (
            <CurrentRow sx={childSpacing.s2} key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        sx={childSpacing.s3}
        label="B. Glass card with hover (rounded-lg, /30 -> /50)"
        note="Matches InfoCard radius, design-system hover transitions"
      >
        <div>
          {ROW_SAMPLES.map((s) => (
            <GlassRow sx={childSpacing.s2} key={s.name} sample={s} />
          ))}
        </div>
      </Variant>

      <Variant
        sx={childSpacing.s3}
        label="C. Compact dense single-line"
        note="One line, leading icon, brand divider, badge tail"
      >
        <div sx={stylexStyles7.rule1}>
          {ROW_SAMPLES.map((s, index) => (
            <DenseRow
              divided={index < ROW_SAMPLES.length - 1}
              key={s.name}
              sample={s}
            />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentRow({
  sample,
  sx,
}: {
  sample: RowSample
  sx?: stylex.StyleXStyles
}) {
  return (
    <div sx={[stylexStyles.rule24, sx]}>
      <div sx={stylexStyles.rule25}>
        <div sx={stylexStyles.rule26}>{sample.name}</div>
        <div sx={stylexStyles.rule27}>{sample.meta}</div>
      </div>
      <CurrentBadge {...sample.state} />
    </div>
  )
}

function GlassRow({
  sample,
  sx,
}: {
  sample: RowSample
  sx?: stylex.StyleXStyles
}) {
  return (
    <div sx={[[stylexStyles2.rule1, groupMarker], sx]}>
      <div sx={stylexStyles.rule28}>
        <span sx={stylexStyles3.rule1}>
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

function DenseRow({
  sample,
  divided,
}: {
  sample: RowSample
  divided?: boolean
}) {
  return (
    <div sx={[stylexStyles.rule31, divided && rowDividers.regular]}>
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

function StatTileSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
      title="Stat tiles (Job progress, Task counts, Capacity)"
      description="Current viewers stack large numeric tiles using InfoCard. Compact stat groups (billing) and StatCard (session dashboard) are lighter."
    >
      <Variant
        sx={childSpacing.s3}
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
        sx={childSpacing.s3}
        label="B. Compact stat group (billing-style)"
        note="Uppercase label, no card per metric"
      >
        <div sx={stylexStyles.rule37}>
          <CompactStat label="Tasks" value="6/12" detail="50% complete" />
          <CompactStat label="Passes" value="4" detail="Current nonce 4" />
        </div>
      </Variant>

      <Variant
        sx={childSpacing.s3}
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
    <div>
      <div sx={[stylexStyles.rule42, childSpacing.s1]}>{label}</div>
      <div sx={[stylexStyles.rule43, childSpacing.s1]}>{value}</div>
      <div sx={[stylexStyles.rule14, childSpacing.s1]}>{detail}</div>
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
      <div sx={stylexStyles3.rule2}>
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

function EmptyStateSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
      title="Empty states"
      description="Forge viewers use centered hero blocks ('No tasks in job', py-4 text-center). Design system prefers compact single-line states with muted icon + text."
    >
      <Variant
        sx={childSpacing.s3}
        label="A. Current (text-muted-foreground py-4 text-center)"
        note="Centered, no icon, deprecated text-muted-foreground token"
      >
        <div sx={stylexStyles.rule48}>No tasks in job</div>
      </Variant>

      <Variant
        sx={childSpacing.s3}
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

function TabBarSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  const [activeA, setActiveA] = useState(0)
  const [activeB, setActiveB] = useState(0)
  const [activeC, setActiveC] = useState(0)

  return (
    <Section
      sx={sx}
      title="Tab bar (ForgeViewerShell)"
      description="Current bar uses bg-foreground underline. Brand-tinted underline ties it to the rest of the UI; pill tabs read better when there are 3+ short labels."
    >
      <Variant
        sx={childSpacing.s3}
        label="A. Current (foreground underline, h-8 row)"
        note="From web/forge/ForgeViewerShell.tsx"
      >
        <CurrentTabs active={activeA} onSelect={setActiveA} />
      </Variant>

      <Variant
        sx={childSpacing.s3}
        label="B. Brand underline + softer inactive"
        note="bg-brand/80 underline, text-foreground-alt/50 -> /80 inactive"
      >
        <BrandTabs active={activeB} onSelect={setActiveB} />
      </Variant>

      <Variant
        sx={childSpacing.s3}
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

function ViewerShellSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  const [tab, setTab] = useState(0)
  return (
    <Section
      sx={sx}
      title="Combined viewer mock"
      description="Full ForgeViewerShell layout using the candidate variants: brand underline tabs, tinted state badge, glass entity rows, compact empty state. Compare against ForgeJobViewer in repos/spacewave/app/forge/."
    >
      <div sx={[stylexStyles.rule64, childSpacing.s3]}>
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

        <div sx={stylexStyles4.rule4}>
          {tab === 0 && (
            <>
              <div sx={[stylexStyles.rule35, childSpacing.s3]}>
                <InlineStat
                  icon={LuListTodo}
                  label="Tasks complete"
                  value="6 of 12"
                />
                <InlineStat icon={LuPlay} label="Passes" value="4" />
              </div>
              <div sx={[stylexStyles.rule70, childSpacing.s3]}>
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
            <div sx={childSpacing.s3}>
              {ROW_SAMPLES.map((s) => (
                <GlassRow sx={childSpacing.s2} key={s.name} sample={s} />
              ))}
            </div>
          )}
          {tab === 2 && (
            <div sx={[stylexStyles.rule7, childSpacing.s3]}>
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
