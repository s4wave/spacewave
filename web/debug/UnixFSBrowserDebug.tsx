import { useCallback, useState, type ReactNode } from 'react'
import {
  LuArrowLeft,
  LuChevronLeft,
  LuChevronRight,
  LuChevronUp,
  LuCircleAlert,
  LuFile,
  LuFolder,
  LuFolderOpen,
  LuFolderPlus,
  LuRotateCw,
  LuUpload,
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
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
  },
  rule16: { backgroundColor: themeColors['--color-file-back'] },
  rule17: {
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 30)`,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'hidden',
  },
  rule18: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-file-folder-icon'],
  },
  rule19: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule20: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule21: {
    width: `calc(${themeLayout['--spacing']} * 35)`,
    minWidth: `calc(${themeLayout['--spacing']} * 25)`,
    flexShrink: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    opacity: 0.7,
  },
  rule22: {
    width: `calc(${themeLayout['--spacing']} * 17.5)`,
    minWidth: `calc(${themeLayout['--spacing']} * 12.5)`,
    flexShrink: 1,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    opacity: 0.7,
  },
  rule23: {
    position: 'absolute',
    top: themeLayout['--spacing'],
    bottom: themeLayout['--spacing'],
    left: 0,
    width: `calc(${themeLayout['--spacing']} * .5)`,
    borderTopRightRadius: '.25rem',
    borderBottomRightRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule24: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule25: { color: themeColors['--color-brand'] },
  rule26: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule27: { color: themeColors['--color-foreground'] },
  rule28: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule29: {
    width: `calc(${themeLayout['--spacing']} * 35)`,
    minWidth: `calc(${themeLayout['--spacing']} * 25)`,
    flexShrink: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule30: {
    width: `calc(${themeLayout['--spacing']} * 17.5)`,
    minWidth: `calc(${themeLayout['--spacing']} * 12.5)`,
    flexShrink: 1,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule31: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule32: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule33: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule34: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule35: {
    display: { default: 'none', '@media (min-width: 40rem)': 'block' },
    width: `calc(${themeLayout['--spacing']} * 30)`,
    flexShrink: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule36: {
    width: `calc(${themeLayout['--spacing']} * 15)`,
    flexShrink: 1,
    textAlign: 'right',
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule37: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-panel-header'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule38: { display: 'flex' },
  rule39: {
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  rule40: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule41: {
    cursor: 'default',
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    opacity: 0.4,
  },
  rule42: {
    marginInline: themeLayout['--spacing'],
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule43: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule44: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
  },
  rule45: { display: 'flex', gap: themeLayout['--spacing'] },
  rule46: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule47: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule48: {
    marginInline: themeLayout['--spacing'],
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: '#0000',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule49: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule50: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule51: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule52: {
    cursor: 'default',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule53: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule54: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 40)`,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-file-back'],
  },
  rule55: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    textAlign: 'center',
  },
  rule56: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule57: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule58: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule59: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 40)`,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-file-back'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule60: {
    width: '100%',
    maxWidth: 'var(--container-xs)',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule61: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule62: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule63: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule64: { minWidth: 0 },
  rule65: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule66: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule67: {
    marginTop: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule68: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule69: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule70: {
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 45)`,
    backgroundColor: themeColors['--color-file-back'],
  },
  rule71: {
    pointerEvents: 'none',
    position: 'absolute',
    inset: 0,
    zIndex: 10,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'dashed',
    borderWidth: '2px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule72: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule73: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-brand'],
  },
  rule74: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule75: {
    pointerEvents: 'none',
    position: 'absolute',
    inset: 0,
    zIndex: 10,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule76: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule77: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
  },
  rule78: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
  rule79: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule80: { opacity: 0.6 },
  rule81: {
    display: 'flex',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule82: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule83: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule84: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 40)`,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-file-back'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule85: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule86: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
    textDecorationLine: 'underline',
  },
  rule87: {
    width: '100%',
    maxWidth: 'var(--container-sm)',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule88: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule89: { minWidth: 0, flex: 1 },
  rule90: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule91: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    justifyContent: 'flex-end',
  },
  rule92: {
    display: 'inline-flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule93: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule94: {
    minHeight: `calc(${themeLayout['--spacing']} * 40)`,
    backgroundColor: themeColors['--color-file-back'],
  },
  rule95: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule96: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule97: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
  },
  rule98: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
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
  rule99: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule100: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-accent'],
      },
    },
  },
  rule101: {
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 90%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive-foreground'],
  },
  rule102: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
  rule103: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

// UnixFSBrowserDebug renders prototype variants for the UnixFS browser
// surfaces (file row, toolbar, empty state, drag overlay, error state,
// destructive dialog). Each section shows the current production look first,
// then candidate variants aligned with the app design system.
export function UnixFSBrowserDebug() {
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
          <LuArrowLeft {...stylex.props(stylexStyles.rule4)} />
        </button>
        <span sx={stylexStyles.rule5}>UnixFS Browser UI Prototype</span>
      </div>

      <div sx={stylexStyles.rule6}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-6 */ 'mx-auto max-w-3xl space-y-6'
          }
        >
          <Intro />
          <FileRowSection />
          <ToolbarSection />
          <EmptyStateSection />
          <DragOverlaySection />
          <ErrorStateSection />
          <DestructiveDialogSection />
        </div>
      </div>
    </div>
  )
}

function Intro() {
  return (
    <div sx={stylexStyles.rule7}>
      <p sx={stylexStyles.rule8}>UnixFS browser variant gallery</p>
      <p sx={stylexStyles.rule9}>
        Side-by-side variants for the surfaces that compose UnixFSBrowser (file
        row, toolbar, empty state, drag overlay, error state, delete dialog).
        Each section shows the current production rendering, then candidate
        variants drawn from the modern token set.
      </p>
      <p sx={stylexStyles.rule10}>Decisions live in production code:</p>
      <ul
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-0.5 */ 'text-foreground-alt/70 mt-1 list-disc space-y-0.5 pl-5 text-xs leading-relaxed'
        }
      >
        <li>
          File row: B (compact dense, modern tokens, brand left bar on selected)
          in web/editors/file-browser/FileListEntry.tsx.
        </li>
        <li>
          Toolbar: B (modern, NavIconButton-style buttons) in
          web/editors/file-browser/Toolbar.tsx.
        </li>
        <li>Empty state: A (current centered three-line) kept as-is.</li>
        <li>Drag overlay: A (current dashed border) kept as-is.</li>
        <li>
          Error state: C (inline strip under toolbar) in
          app/unixfs/UnixFSBrowser.tsx.
        </li>
        <li>
          Delete dialog buttons: B (tinted destructive outline) in
          app/unixfs/UnixFSBrowser.tsx.
        </li>
      </ul>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section / Variant wrappers
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
// File row
// -----------------------------------------------------------------------

interface FileRowSample {
  id: string
  name: string
  isDir: boolean
  modTime: string
  size: string
}

const FILE_ROWS: FileRowSample[] = [
  {
    id: 'a',
    name: 'documents',
    isDir: true,
    modTime: 'Apr 28, 2026',
    size: '12 items',
  },
  {
    id: 'b',
    name: 'photos',
    isDir: true,
    modTime: 'Apr 22, 2026',
    size: '3 items',
  },
  {
    id: 'c',
    name: 'spec.md',
    isDir: false,
    modTime: 'Apr 30, 2026',
    size: '4.2 KB',
  },
  {
    id: 'd',
    name: 'screenshot.png',
    isDir: false,
    modTime: 'Apr 18, 2026',
    size: '128 KB',
  },
  {
    id: 'e',
    name: 'archive.tar.gz',
    isDir: false,
    modTime: 'Mar 09, 2026',
    size: '2.4 MB',
  },
]

function FileRowSection() {
  const [selectedA, setSelectedA] = useState<string | null>('c')
  const [selectedB, setSelectedB] = useState<string | null>('c')
  const [selectedC, setSelectedC] = useState<string | null>('c')

  return (
    <Section
      title="File row (FileListEntry)"
      description="Each row in the directory list. Current rows use deprecated tokens, alternating row backgrounds, and a focus ring; design-system rows use the modern token set with hover/selection on /5 and /10 opacities."
    >
      <Variant
        label="A. Current (alternating rows + legacy tokens)"
        note="text-file-browser-row, bg-ui-selected, bg-file-row-alternate"
      >
        <div sx={stylexStyles.rule16}>
          {FILE_ROWS.map((row, i) => (
            <CurrentRow
              key={row.id}
              row={row}
              index={i}
              selected={selectedA === row.id}
              onSelect={() => setSelectedA(row.id)}
            />
          ))}
        </div>
      </Variant>

      <Variant
        label="B. Compact dense (modern tokens, no alternating)"
        note="hover bg-foreground/5, selection bg-brand/10 + brand left bar"
      >
        <div>
          {FILE_ROWS.map((row) => (
            <DenseRow
              key={row.id}
              row={row}
              selected={selectedB === row.id}
              onSelect={() => setSelectedB(row.id)}
            />
          ))}
        </div>
      </Variant>

      <Variant
        label="C. Card rows (rounded, gapped)"
        note="Each row a glass card; readable in lists < 12 entries"
      >
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1.5 */ 'space-y-1.5 p-2'
          }
        >
          {FILE_ROWS.map((row) => (
            <CardRow
              key={row.id}
              row={row}
              selected={selectedC === row.id}
              onSelect={() => setSelectedC(row.id)}
            />
          ))}
        </div>
      </Variant>
    </Section>
  )
}

function CurrentRow({
  row,
  index,
  selected,
  onSelect,
}: {
  row: FileRowSample
  index: number
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={
        /* TODO(stylex): global or unknown class: group */ cn(
          'group text-file-browser-row flex items-center px-3 py-1.5 text-xs',
          'hover:bg-outliner-selected-highlight cursor-pointer transition-colors select-none',
          selected && 'bg-ui-selected hover:bg-ui-selected',
          index % 2 === 1 && !selected && 'bg-file-row-alternate',
        )
      }
    >
      <div sx={stylexStyles.rule17}>
        {row.isDir ? (
          <LuFolder {...stylex.props(stylexStyles.rule18)} />
        ) : (
          <LuFile {...stylex.props(stylexStyles.rule19)} />
        )}
        <span sx={stylexStyles.rule20}>{row.name}</span>
      </div>
      <div sx={stylexStyles.rule21}>{row.modTime}</div>
      <div sx={stylexStyles.rule22}>{row.size}</div>
    </div>
  )
}

function DenseRow({
  row,
  selected,
  onSelect,
}: {
  row: FileRowSample
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={
        /* TODO(stylex): global or unknown class: group */ cn(
          'group relative flex cursor-pointer items-center px-3 py-1.5 text-xs transition-colors select-none',
          selected
            ? 'bg-brand/10 text-foreground'
            : 'text-foreground/90 hover:bg-foreground/5',
        )
      }
    >
      {selected && <span sx={stylexStyles.rule23} />}
      <div sx={stylexStyles.rule17}>
        {row.isDir ? (
          <LuFolder
            {...stylex.props([
              stylexStyles.rule24,
              selected ? stylexStyles.rule25 : stylexStyles.rule26,
            ])}
          />
        ) : (
          <LuFile
            {...stylex.props([
              stylexStyles.rule24,
              selected ? stylexStyles.rule27 : stylexStyles.rule28,
            ])}
          />
        )}
        <span sx={stylexStyles.rule20}>{row.name}</span>
      </div>
      <div sx={stylexStyles.rule29}>{row.modTime}</div>
      <div sx={stylexStyles.rule30}>{row.size}</div>
    </div>
  )
}

function CardRow({
  row,
  selected,
  onSelect,
}: {
  row: FileRowSample
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
      className={
        /* TODO(stylex): global or unknown class: group */ cn(
          'group flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2 text-xs transition-all duration-150 select-none',
          selected
            ? 'border-brand/30 bg-brand/5'
            : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50',
        )
      }
    >
      <span
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:bg-foreground/8 */ cn(
            'flex size-7 shrink-0 items-center justify-center rounded-md transition-colors',
            selected
              ? 'bg-brand/15'
              : 'bg-foreground/5 group-hover:bg-foreground/8',
          )
        }
      >
        {row.isDir ? (
          <LuFolder
            {...stylex.props([
              stylexStyles.rule31,
              selected ? stylexStyles.rule25 : stylexStyles.rule32,
            ])}
          />
        ) : (
          <LuFile
            {...stylex.props([
              stylexStyles.rule31,
              selected ? stylexStyles.rule27 : stylexStyles.rule28,
            ])}
          />
        )}
      </span>
      <div sx={stylexStyles.rule33}>
        <div sx={stylexStyles.rule34}>{row.name}</div>
      </div>
      <div sx={stylexStyles.rule35}>{row.modTime}</div>
      <div sx={stylexStyles.rule36}>{row.size}</div>
    </div>
  )
}

// -----------------------------------------------------------------------
// Toolbar
// -----------------------------------------------------------------------

function ToolbarSection() {
  return (
    <Section
      title="Toolbar (Toolbar.tsx + PanelHeader)"
      description="Top-of-browser nav. Current bar uses bg-panel-header and bg-pulldown-hover button states (legacy). Design-system bars use border-foreground/8 dividers and DashboardButton-style action chips."
    >
      <Variant
        label="A. Current (PanelHeader + bg-pulldown-hover)"
        note="Native button hover, no tooltips on the path bar"
      >
        <CurrentToolbar />
      </Variant>

      <Variant
        label="B. Modern toolbar"
        note="border-foreground/8 row, foreground-alt -> foreground hover, action chips"
      >
        <ModernToolbar />
      </Variant>
    </Section>
  )
}

function CurrentToolbar() {
  return (
    <div sx={stylexStyles.rule37}>
      <div sx={stylexStyles.rule38}>
        <button type="button" aria-label="Back" sx={stylexStyles.rule39}>
          <LuChevronLeft {...stylex.props(stylexStyles.rule40)} />
        </button>
        <button type="button" aria-label="Forward" sx={stylexStyles.rule41}>
          <LuChevronRight {...stylex.props(stylexStyles.rule40)} />
        </button>
        <button type="button" aria-label="Up" sx={stylexStyles.rule39}>
          <LuChevronUp {...stylex.props(stylexStyles.rule40)} />
        </button>
      </div>
      <div sx={stylexStyles.rule42}>
        <LuFolderOpen {...stylex.props(stylexStyles.rule43)} />
        <span sx={stylexStyles.rule44}>/photos/2026/april</span>
      </div>
      <div sx={stylexStyles.rule45}>
        <button type="button" title="New folder" sx={stylexStyles.rule39}>
          <LuFolderPlus {...stylex.props(stylexStyles.rule40)} />
        </button>
        <button type="button" title="Upload files" sx={stylexStyles.rule39}>
          <LuUpload {...stylex.props(stylexStyles.rule40)} />
        </button>
      </div>
    </div>
  )
}

function ModernToolbar() {
  return (
    <div sx={stylexStyles.rule46}>
      <div sx={stylexStyles.rule47}>
        <NavIconButton
          icon={<LuChevronLeft {...stylex.props(stylexStyles.rule31)} />}
          label="Back"
        />
        <NavIconButton
          icon={<LuChevronRight {...stylex.props(stylexStyles.rule31)} />}
          label="Forward"
          disabled
        />
        <NavIconButton
          icon={<LuChevronUp {...stylex.props(stylexStyles.rule31)} />}
          label="Up"
        />
      </div>
      <div sx={stylexStyles.rule48}>
        <LuFolderOpen {...stylex.props(stylexStyles.rule49)} />
        <span sx={stylexStyles.rule50}>/photos/2026/april</span>
      </div>
      <div sx={stylexStyles.rule47}>
        <NavIconButton
          icon={<LuFolderPlus {...stylex.props(stylexStyles.rule31)} />}
          label="New folder"
        />
        <NavIconButton
          icon={<LuUpload {...stylex.props(stylexStyles.rule31)} />}
          label="Upload"
        />
      </div>
    </div>
  )
}

function NavIconButton({
  icon,
  label,
  disabled,
}: {
  icon: ReactNode
  label: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      sx={[
        stylexStyles.rule51,
        disabled ? stylexStyles.rule52 : stylexStyles.rule53,
      ]}
    >
      {icon}
    </button>
  )
}

// -----------------------------------------------------------------------
// Empty states
// -----------------------------------------------------------------------

function EmptyStateSection() {
  return (
    <Section
      title="Empty state (no UnixFS object)"
      description="Shown when rootHandle has no value. Current state stacks three muted lines centered on bg-file-back. Design-system pattern is a compact card with muted icon + headline."
    >
      <Variant
        label="A. Current (centered three-line)"
        note="text-foreground-alt + text-foreground-alt/70, no icon"
      >
        <div sx={stylexStyles.rule54}>
          <div sx={stylexStyles.rule55}>
            <div sx={stylexStyles.rule56}>UnixFS object not found</div>
            <div sx={stylexStyles.rule57}>Object: 0x0a1b2c3d4e5f…</div>
            <div sx={stylexStyles.rule58}>
              Create a drive via quickstart to initialize demo content.
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="B. Compact card (design-system)"
        note="border-foreground/6 bg-background-card/30, muted icon + headline + helper"
      >
        <div sx={stylexStyles.rule59}>
          <div sx={stylexStyles.rule60}>
            <div sx={stylexStyles.rule61}>
              <span sx={stylexStyles.rule62}>
                <LuFolder {...stylex.props(stylexStyles.rule63)} />
              </span>
              <div sx={stylexStyles.rule64}>
                <p sx={stylexStyles.rule65}>UnixFS object not found</p>
                <p sx={stylexStyles.rule66}>
                  Create a drive via quickstart to initialize demo content.
                </p>
                <p sx={stylexStyles.rule67}>0x0a1b2c3d4e5f…</p>
              </div>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="C. Single-line muted"
        note="Matches design-system 'compact empty state', text-foreground-alt/40"
      >
        <div sx={stylexStyles.rule59}>
          <div sx={stylexStyles.rule68}>
            <LuFolder {...stylex.props(stylexStyles.rule69)} />
            <span>No UnixFS object yet. Create a drive via quickstart.</span>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Drag overlay
// -----------------------------------------------------------------------

function DragOverlaySection() {
  return (
    <Section
      title="Drag overlay (drop-to-upload)"
      description="Shown over the file list while files are being dragged in. Current overlay uses border-2 border-dashed and a large icon. Design-system tightens the chrome and uses the glass stack."
    >
      <Variant
        label="A. Current (large dashed border + h-8 icon)"
        note="border-brand/50, bg-brand/5, text-sm"
      >
        <div sx={stylexStyles.rule70}>
          <DummyRows />
          <div sx={stylexStyles.rule71}>
            <div sx={stylexStyles.rule72}>
              <LuUpload {...stylex.props(stylexStyles.rule73)} />
              <span sx={stylexStyles.rule74}>Drop files to upload</span>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="B. Glass card overlay"
        note="border-brand/30 dashed, glass background, compact icon + caption"
      >
        <div sx={stylexStyles.rule70}>
          <DummyRows />
          <div sx={stylexStyles.rule75}>
            <div sx={stylexStyles.rule76}>
              <span sx={stylexStyles.rule77}>
                <LuUpload {...stylex.props(stylexStyles.rule78)} />
              </span>
              <div sx={stylexStyles.rule64}>
                <p sx={stylexStyles.rule65}>Drop files to upload</p>
                <p sx={stylexStyles.rule79}>to /photos/2026/april</p>
              </div>
            </div>
          </div>
        </div>
      </Variant>
    </Section>
  )
}

function DummyRows() {
  return (
    <div sx={stylexStyles.rule80}>
      {FILE_ROWS.slice(0, 4).map((row) => (
        <div key={row.id} sx={stylexStyles.rule81}>
          <div sx={stylexStyles.rule17}>
            {row.isDir ? (
              <LuFolder {...stylex.props(stylexStyles.rule82)} />
            ) : (
              <LuFile {...stylex.props(stylexStyles.rule82)} />
            )}
            <span sx={stylexStyles.rule83}>{row.name}</span>
          </div>
        </div>
      ))}
    </div>
  )
}

// -----------------------------------------------------------------------
// Error state
// -----------------------------------------------------------------------

function ErrorStateSection() {
  return (
    <Section
      title="Error state (load failure)"
      description="Shown when any of root/path/stat/entries resources error. Current state is plain text + an underline retry link; design-system patterns use a tinted destructive callout with a button."
    >
      <Variant
        label="A. Current (plain text + underline link)"
        note="text-destructive headline, text-brand underline retry"
      >
        <div sx={stylexStyles.rule84}>
          <div sx={stylexStyles.rule85}>Error loading files</div>
          <div sx={stylexStyles.rule57}>
            failed to read directory: permission denied
          </div>
          <button type="button" sx={stylexStyles.rule86}>
            Retry
          </button>
        </div>
      </Variant>

      <Variant
        label="B. Tinted destructive callout"
        note="border-destructive/20 bg-destructive/5 + LuCircleAlert + retry chip"
      >
        <div sx={stylexStyles.rule59}>
          <div sx={stylexStyles.rule87}>
            <div sx={stylexStyles.rule61}>
              <LuCircleAlert {...stylex.props(stylexStyles.rule88)} />
              <div sx={stylexStyles.rule89}>
                <p sx={stylexStyles.rule65}>Error loading files</p>
                <p sx={stylexStyles.rule90}>
                  failed to read directory: permission denied
                </p>
              </div>
            </div>
            <div sx={stylexStyles.rule91}>
              <button type="button" sx={stylexStyles.rule92}>
                <LuRotateCw {...stylex.props(stylexStyles.rule93)} />
                Retry
              </button>
            </div>
          </div>
        </div>
      </Variant>

      <Variant
        label="C. Inline strip (one-line)"
        note="Slim banner, fits under the toolbar without disrupting the file list"
      >
        <div sx={stylexStyles.rule94}>
          <div sx={stylexStyles.rule95}>
            <LuCircleAlert {...stylex.props(stylexStyles.rule96)} />
            <p sx={stylexStyles.rule97}>
              Error loading files: permission denied
            </p>
            <button type="button" sx={stylexStyles.rule98}>
              <LuRotateCw {...stylex.props(stylexStyles.rule93)} />
              Retry
            </button>
          </div>
          <DummyRows />
        </div>
      </Variant>
    </Section>
  )
}

// -----------------------------------------------------------------------
// Destructive (delete) dialog buttons
// -----------------------------------------------------------------------

function DestructiveDialogSection() {
  return (
    <Section
      title="Delete confirmation buttons"
      description="DialogFooter buttons for the delete dialog. Current uses a solid destructive fill ('bg-destructive text-destructive-foreground'); design-system primary actions use tinted outlines with /30 borders and /10 fills."
    >
      <Variant
        label="A. Current (solid destructive)"
        note="bg-destructive + text-destructive-foreground"
      >
        <div sx={stylexStyles.rule99}>
          <button type="button" sx={stylexStyles.rule100}>
            Cancel
          </button>
          <button type="button" sx={stylexStyles.rule101}>
            Delete
          </button>
        </div>
      </Variant>

      <Variant
        label="B. Tinted destructive outline"
        note="border-destructive/30 bg-destructive/10 hover:bg-destructive/15"
      >
        <div sx={stylexStyles.rule99}>
          <button type="button" sx={stylexStyles.rule102}>
            Cancel
          </button>
          <button type="button" sx={stylexStyles.rule103}>
            Delete
          </button>
        </div>
      </Variant>
    </Section>
  )
}
