import {
  useEffect,
  useRef,
  type ComponentProps,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import {
  LuArrowDown,
  LuArrowLeft,
  LuArrowRight,
  LuArrowUp,
  LuBox,
  LuCircleAlert,
  LuCpu,
  LuFolderOpen,
  LuHardDrive,
  LuLayers,
  LuRadar,
  LuRefreshCw,
  LuRocket,
  LuTriangleAlert,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import {
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useTrackedResources } from '@s4wave/web/devtools/index.js'
import { useStateInspectorEntryMap } from '@s4wave/web/devtools/useStateInspectorEntries.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { NetworkInspector } from './NetworkInspector.js'
import { ReleaseInspector } from './ReleaseInspector.js'
import { RuntimeInspector, type RuntimeView } from './RuntimeInspector.js'
import { SpacesInspector } from './SpacesInspector.js'
import { StorageInspector } from './StorageInspector.js'
import { StorageMeter } from './StorageMeter.js'
import { SyncInspector } from './SyncInspector.js'
import { SystemTile } from './SystemTile.js'
import { UnderTheHoodInspector } from './UnderTheHoodInspector.js'
import {
  formatBytes,
  formatCount,
  plural,
  protectionLabel,
  shortId,
} from './format.js'
import { toneDotStyles, toneTextStyles } from './tone.js'
import {
  spaceEngineId,
  updatePhaseLabel,
  useSystemModel,
  type AttentionItem,
  type SubsystemId,
  type SystemModel,
  type SystemTone,
} from './useSystemModel.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    flexShrink: 0,
    borderRadius: '3.40282e38px',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background'],
  },
  rule2: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    overflow: 'auto',
  },
  rule3: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    animationName: {
      default: 'shadcn-enter',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: '.15s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: 'ease-out',
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: 1,
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: 'normal',
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: 'running',
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: '0s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: 'both',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: 'auto',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-duration': '.2s',
    transitionDuration: '.2s',
    '--tw-ease': 'var(--ease-out)',
    transitionTimingFunction: 'var(--ease-out)',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-enter-opacity': '0',
    '--tw-enter-scale': '.95',
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule5: { minHeight: 0, flex: 1, overflow: 'auto' },
  rule6: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule8: {
    display: { default: 'none', '@container (min-width: 32rem)': 'block' },
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule10: {
    marginLeft: 'auto',
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 7)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 7)`,
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
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
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule11: { minWidth: 0, flex: 1 },
  rule12: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule14: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule15: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@container (min-width: 42rem)': 'repeat(6, minmax(0, 1fr))',
    },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule16: {
    gridColumn: {
      default: null,
      '@container (min-width: 42rem)': 'span 2 / span 2',
    },
  },
  rule17: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule18: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule19: {
    marginTop: themeLayout['--spacing'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule20: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule21: { marginBlock: `calc(${themeLayout['--spacing']} * 2)` },
  rule22: { color: themeColors['--color-warning'] },
  rule23: {
    gridColumn: {
      default: null,
      '@container (min-width: 42rem)': 'span 4 / span 4',
    },
  },
  rule24: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule25: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule26: {
    height: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 20)`,
    flexShrink: 0,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule27: {
    width: 'var(--progress-width)',
    display: 'block',
    height: '100%',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule28: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule29: { color: themeColors['--color-destructive'] },
  rule30: { marginTop: themeLayout['--spacing'] },
  rule31: {
    gridColumn: {
      default: null,
      '@container (min-width: 42rem)': 'span 6 / span 6',
    },
  },
  rule32: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule33: {
    maxWidth: `calc(${themeLayout['--spacing']} * 48)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule34: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule35: {
    display: 'flex',
    maxWidth: `calc(${themeLayout['--spacing']} * 56)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule36: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule37: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule38: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule39: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule40: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule41: {
    marginRight: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule42: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule43: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    overflowX: 'auto',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule44: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
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
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule45: {
    marginInline: themeLayout['--spacing'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: '1px',
    flexShrink: 0,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule46: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule47: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule48: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule49: {
    marginLeft: 'auto',
    display: { default: 'none', '@container (min-width: 36rem)': 'block' },
    flexShrink: 0,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule50: { fontFamily: 'var(--font-sans)' },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 25%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// InspectorId names a full-size inspector: one per subsystem tile plus the
// two devtools trees.
type InspectorId = SubsystemId | 'resources' | 'atoms'

// TileRef registers the control that opens an inspector, so focus can return
// to it when the inspector closes.
type TileRef = (id: InspectorId) => (node: HTMLButtonElement | null) => void

// inspectorMeta titles and describes every inspector in navigation order.
const inspectorMeta: Record<
  InspectorId,
  { title: string; icon: ReactNode; description: string }
> = {
  sync: {
    title: 'Sync',
    icon: <LuRefreshCw />,
    description: 'How this session moves blocks to and from your other copies.',
  },
  storage: {
    title: 'Storage',
    icon: <LuHardDrive />,
    description: 'Where your data lives on this device and how safe it is.',
  },
  network: {
    title: 'Network',
    icon: <LuRadar />,
    description: 'The encrypted peer links this device holds right now.',
  },
  runtime: {
    title: 'Runtime',
    icon: <LuCpu />,
    description:
      'Plugins, controllers, and directives running on the session bus.',
  },
  release: {
    title: 'Release',
    icon: <LuRocket />,
    description: 'This build, its updates, and how it booted.',
  },
  spaces: {
    title: 'Spaces & Accounts',
    icon: <LuFolderOpen />,
    description: 'Every account on this device and the Spaces it holds.',
  },
  resources: {
    title: 'Resources',
    icon: <LuLayers />,
    description: 'SDK resources this app holds open, as a live tree.',
  },
  atoms: {
    title: 'State atoms',
    icon: <LuBox />,
    description: 'Persisted UI state, as a live tree.',
  },
}

const inspectorOrder = Object.keys(inspectorMeta) as InspectorId[]

interface SystemStatusDashboardProps {
  onClose?: () => void
}

// SystemStatusDashboard is the system mission control overlay. Every
// subsystem shows as a live tile under a one-line verdict; a tile opens its
// full inspector in place, and Escape returns to the tiles.
export function SystemStatusDashboard({ onClose }: SystemStatusDashboardProps) {
  const ns = useStateNamespace(['system-status-dashboard'])
  const [inspector, setInspector] = useStateAtom<InspectorId | ''>(
    ns,
    'inspector',
    '',
  )
  const [runtimeView, setRuntimeView] = useStateAtom<RuntimeView>(
    ns,
    'runtime-view',
    'plugins',
  )
  const model = useSystemModel()
  const navigateSession = useSessionNavigate()

  const tileRefs = useRef<Partial<Record<InspectorId, HTMLButtonElement>>>({})
  const inspectorRef = useRef<HTMLDivElement>(null)
  const lastInspectorRef = useRef<InspectorId | ''>('')

  // Move focus into an opened inspector, and back to its tile on return.
  useEffect(() => {
    if (inspector) {
      lastInspectorRef.current = inspector
      inspectorRef.current?.focus({ preventScroll: true })
      return
    }
    const last = lastInspectorRef.current
    if (last) {
      tileRefs.current[last]?.focus()
    }
  }, [inspector])

  const close = () => onClose?.()

  // Escape leaves an open inspector before it reaches the overlay frame.
  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Escape' || !inspector) {
      return
    }
    event.preventDefault()
    event.stopPropagation()
    setInspector('')
  }

  const tileRef: TileRef = (id) => (node) => {
    if (node) {
      tileRefs.current[id] = node
    } else {
      delete tileRefs.current[id]
    }
  }

  return (
    <div
      role="region"
      aria-label="System status"
      sx={stylexStyles3.rule1}
      onKeyDown={handleKeyDown}
    >
      <VerdictBar model={model} onClose={close} />

      {inspector ? (
        <>
          <InspectorNav
            current={inspector}
            tones={model.tones}
            onSelect={setInspector}
            onBack={() => setInspector('')}
          />
          <div sx={stylexStyles3.rule2}>
            <div
              key={inspector}
              ref={inspectorRef}
              tabIndex={-1}
              aria-labelledby="system-inspector-title"
              sx={stylexStyles3.rule3}
            >
              <div>
                <h2
                  id="system-inspector-title"
                  className={
                    /* TODO(stylex): a descendant, group, peer or compound selector: [&>svg]:size-4 */ 'text-foreground flex items-center gap-2 text-lg font-semibold tracking-tight [&>svg]:size-4'
                  }
                >
                  {inspectorMeta[inspector].icon}
                  {inspectorMeta[inspector].title}
                </h2>
                <p sx={stylexStyles3.rule4}>
                  {inspectorMeta[inspector].description}
                </p>
              </div>

              {inspector === 'sync' && <SyncInspector sync={model.sync} />}
              {inspector === 'storage' && (
                <StorageInspector
                  storage={model.storage}
                  onOpenSettings={() => {
                    navigateSession({ path: 'settings/storage' })
                    close()
                  }}
                />
              )}
              {inspector === 'network' && (
                <NetworkInspector network={model.network} />
              )}
              {inspector === 'runtime' && (
                <RuntimeInspector
                  plugins={model.plugins}
                  controllers={model.controllers}
                  directives={model.directives}
                  directiveTree={model.directiveTree}
                  directiveCount={model.directiveCount}
                  view={runtimeView}
                  onViewChange={setRuntimeView}
                />
              )}
              {inspector === 'release' && (
                <ReleaseInspector
                  build={model.build}
                  recovery={model.recovery}
                />
              )}
              {inspector === 'spaces' && (
                <SpacesInspector
                  sessions={model.sessions}
                  spaces={model.spaces}
                  onClose={close}
                />
              )}
              {(inspector === 'resources' || inspector === 'atoms') && (
                <UnderTheHoodInspector kind={inspector} />
              )}
            </div>
          </div>
        </>
      ) : (
        <div sx={stylexStyles3.rule5}>
          <AttentionList items={model.verdict.items} onOpen={setInspector} />
          <SystemGrid model={model} tileRef={tileRef} onOpen={setInspector} />
          <UnderTheHood tileRef={tileRef} onOpen={setInspector} />
        </div>
      )}
    </div>
  )
}

// VerdictBar is the one-line system verdict with the build identity.
function VerdictBar({
  model,
  onClose,
}: {
  model: SystemModel
  onClose: () => void
}) {
  const { build, verdict } = model
  const platform =
    build.goos && build.goarch ? `${build.goos}/${build.goarch}` : ''

  return (
    <header sx={stylexStyles3.rule6}>
      <span
        sx={[[stylexStyles5.rule1], [toneDotStyles[verdict.tone]]]}
        aria-hidden="true"
      />
      <h1 sx={stylexStyles3.rule7} role="status">
        {verdict.headline}
      </h1>
      <p sx={stylexStyles3.rule8}>
        <span sx={stylexStyles3.rule9}>{build.version || 'dev'}</span>
        {build.runtimeLabel && <> · {build.runtimeLabel}</>}
        {platform && (
          <>
            {' · '}
            <span sx={stylexStyles3.rule9}>{platform}</span>
          </>
        )}
      </p>
      <button
        type="button"
        onClick={onClose}
        aria-label="Close system status"
        sx={stylexStyles3.rule10}
      >
        <LuX sx={stylexStyles.rule1} aria-hidden="true" />
      </button>
    </header>
  )
}

// AttentionList lists every live fault or risk, each linked to the inspector
// that explains it.
function AttentionList({
  items,
  onOpen,
}: {
  items: AttentionItem[]
  onOpen: (id: InspectorId) => void
}) {
  if (!items.length) {
    return null
  }

  return (
    <ul sx={stylexStyles4.rule1} aria-label="Needs attention">
      {items.map((item) => (
        <li sx={nativeChildSpacing.s2} key={item.key}>
          <button
            type="button"
            onClick={() => onOpen(item.subsystem)}
            sx={[
              [stylexStyles2.rule1, groupMarker],
              item.tone === 'error' ? stylexStyles2.rule2 : stylexStyles2.rule3,
            ]}
          >
            {item.tone === 'error' ? (
              <LuCircleAlert sx={stylexStyles.rule2} aria-hidden="true" />
            ) : (
              <LuTriangleAlert sx={stylexStyles.rule3} aria-hidden="true" />
            )}
            <span sx={stylexStyles3.rule11}>
              <span sx={stylexStyles3.rule12}>{item.title}</span>
              <span sx={stylexStyles3.rule13}>{item.detail}</span>
            </span>
            <span sx={stylexStyles3.rule14}>
              Inspect {inspectorMeta[item.subsystem].title}
              <LuArrowRight sx={stylexStyles.rule4} aria-hidden="true" />
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

// TileBase is the identity and open action every subsystem tile shares.
type TileBase = Pick<
  ComponentProps<typeof SystemTile>,
  'title' | 'icon' | 'tone' | 'buttonRef' | 'onOpen'
>

// SystemGrid lays out the six live subsystem tiles.
function SystemGrid({
  model,
  tileRef,
  onOpen,
}: {
  model: SystemModel
  tileRef: TileRef
  onOpen: (id: InspectorId) => void
}) {
  const tile = (id: SubsystemId): TileBase => ({
    title: inspectorMeta[id].title,
    icon: inspectorMeta[id].icon,
    tone: model.tones[id],
    buttonRef: tileRef(id),
    onOpen: () => onOpen(id),
  })

  return (
    <div sx={stylexStyles3.rule15}>
      <SyncTile base={tile('sync')} sync={model.sync} />
      <StorageTile base={tile('storage')} storage={model.storage} />
      <NetworkTile base={tile('network')} network={model.network} />
      <RuntimeTile base={tile('runtime')} model={model} />
      <ReleaseTile base={tile('release')} model={model} />
      <SpacesTile
        base={tile('spaces')}
        sessions={model.sessions}
        spaces={model.spaces}
      />
    </div>
  )
}

// SyncTile shows the sync summary, live transfer rates, and transport.
function SyncTile({
  base,
  sync,
}: {
  base: TileBase
  sync: SystemModel['sync']
}) {
  return (
    <SystemTile
      {...base}
      sx={stylexStyles3.rule16}
      headline={sync.summaryLabel}
    >
      <div sx={stylexStyles3.rule17}>
        <span sx={stylexStyles3.rule18}>
          <LuArrowUp sx={stylexStyles.rule5} aria-label="Upload" />
          {sync.uploadRateLabel}
        </span>
        <span sx={stylexStyles3.rule18}>
          <LuArrowDown sx={stylexStyles.rule5} aria-label="Download" />
          {sync.downloadRateLabel}
        </span>
      </div>
      <p sx={stylexStyles3.rule19}>{sync.transportLabel}</p>
    </SystemTile>
  )
}

// StorageTile shows the local store size, browser quota, and protection.
function StorageTile({
  base,
  storage,
}: {
  base: TileBase
  storage: SystemModel['storage']
}) {
  let headline = 'No reading'
  if (storage.providerLoading) {
    headline = 'Reading…'
  } else if (storage.providerSupported) {
    headline = formatBytes(storage.providerBytes)
  }

  return (
    <SystemTile {...base} sx={stylexStyles3.rule16} headline={headline}>
      <p sx={stylexStyles3.rule20}>
        {formatCount(storage.blockCount)} block entries
      </p>
      <div sx={stylexStyles3.rule21}>
        <StorageMeter
          usage={storage.originUsageBytes}
          quota={storage.originQuotaBytes}
          compact
        />
      </div>
      <p>Cleanup protection: {protectionLabel(storage.protectionState)}</p>
    </SystemTile>
  )
}

// NetworkTile shows peers, links, transport state, and this device's peer.
function NetworkTile({
  base,
  network,
}: {
  base: TileBase
  network: SystemModel['network']
}) {
  if (!network) {
    return (
      <SystemTile {...base} sx={stylexStyles3.rule16} headline="Reading…" />
    )
  }

  return (
    <SystemTile
      {...base}
      sx={stylexStyles3.rule16}
      headline={plural(network.peerCount ?? 0, 'peer')}
    >
      <p>
        {plural(network.linkCount ?? 0, 'link')} · Transport{' '}
        <span sx={[!network.transportRunning && stylexStyles3.rule22]}>
          {network.transportRunning ? 'running' : 'stopped'}
        </span>
      </p>
      {network.localPeerId && (
        <p sx={stylexStyles3.rule19}>
          This device{' '}
          <span sx={stylexStyles3.rule9}>{shortId(network.localPeerId)}</span>
        </p>
      )}
    </SystemTile>
  )
}

// runtimeHeadline summarizes the plugins running across every plugin host.
function runtimeHeadline(plugins: SystemModel['plugins'], running: number) {
  if (!plugins) return 'Reading…'
  if (plugins.length === 0) return 'No plugins running'
  const headline = `${formatCount(running)} of ${plural(plugins.length, 'plugin')} running`
  const spaces = new Set(
    plugins.filter((plugin) => plugin.spaceId).map((plugin) => plugin.spaceId),
  ).size
  return spaces ? `${headline} in ${plural(spaces, 'Space')}` : headline
}

// RuntimeTile shows plugin health and the busiest directive types.
function RuntimeTile({ base, model }: { base: TileBase; model: SystemModel }) {
  const plugins = model.plugins ?? []
  const running = plugins.filter((plugin) => plugin.state === 'running').length
  const busiest = (model.directives ?? []).slice(0, 3)

  return (
    <SystemTile
      {...base}
      sx={stylexStyles3.rule23}
      headline={runtimeHeadline(model.plugins, running)}
    >
      <p sx={stylexStyles3.rule20}>
        {plural(model.controllers?.length ?? 0, 'controller')} ·{' '}
        {plural(model.directiveCount, 'directive')}
      </p>
      {busiest.length > 0 && (
        <ol sx={stylexStyles4.rule2} aria-label="Busiest directive types">
          {busiest.map((group) => (
            <li
              key={group.name}
              sx={[nativeChildSpacing.s1, stylexStyles3.rule24]}
            >
              <span sx={stylexStyles3.rule25}>{group.name}</span>
              <span sx={stylexStyles3.rule26} aria-hidden="true">
                <span
                  sx={stylexStyles3.rule27}
                  style={{
                    '--progress-width': `${(group.idents.length / busiest[0].idents.length) * 100}%`,
                  }}
                />
              </span>
              <span sx={stylexStyles3.rule28}>
                {formatCount(group.idents.length)}
              </span>
            </li>
          ))}
        </ol>
      )}
    </SystemTile>
  )
}

// ReleaseTile shows this version, the update phase, and the runtime load.
function ReleaseTile({ base, model }: { base: TileBase; model: SystemModel }) {
  const launcher = model.recovery?.launcher
  const asset = model.recovery?.runtimeAsset
  let update = 'No launcher in this runtime'
  if (!model.recovery) {
    update = 'Reading…'
  } else if (launcher) {
    update = updatePhaseLabel(launcher.updatePhase)
  }

  return (
    <SystemTile
      {...base}
      sx={stylexStyles3.rule16}
      headline={
        <span sx={stylexStyles3.rule9}>{model.build.version || 'dev'}</span>
      }
    >
      <p sx={[launcher?.updatePhase === 'error' && stylexStyles3.rule29]}>
        {update}
      </p>
      {asset?.status === 'reported' && (
        <p sx={[stylexStyles3.rule30, !asset.ok && stylexStyles3.rule29]}>
          {asset.ok ? 'Runtime loaded cleanly' : 'Runtime loaded with errors'}
        </p>
      )}
    </SystemTile>
  )
}

// SpacesTile shows every account session and the first Spaces it holds.
function SpacesTile({
  base,
  sessions,
  spaces,
}: {
  base: TileBase
  sessions: SystemModel['sessions']
  spaces: SystemModel['spaces']
}) {
  if (!sessions || !spaces) {
    return (
      <SystemTile {...base} sx={stylexStyles3.rule31} headline="Reading…" />
    )
  }

  const shown = spaces.slice(0, 8)
  return (
    <SystemTile
      {...base}
      sx={stylexStyles3.rule31}
      headline={`${plural(spaces.length, 'Space')} · ${plural(sessions.length, 'account')}`}
    >
      <ul sx={stylexStyles3.rule32}>
        {sessions.map((session) => (
          <SessionChip
            key={session.sessionIndex}
            sessionIndex={session.sessionIndex ?? 0}
          />
        ))}
        {shown.map((space) => (
          <li key={spaceEngineId(space)} sx={stylexStyles3.rule33}>
            {space.spaceMeta?.name || 'Untitled Space'}
          </li>
        ))}
        {spaces.length > shown.length && (
          <li sx={stylexStyles3.rule34}>
            +{formatCount(spaces.length - shown.length)} more
          </li>
        )}
      </ul>
    </SystemTile>
  )
}

// SessionChip names one account session, marking the current one.
function SessionChip({ sessionIndex }: { sessionIndex: number }) {
  const metadata = useSessionMetadata(sessionIndex)
  const current = useSessionIndex() === sessionIndex

  return (
    <li
      sx={[
        stylexStyles3.rule35,
        current ? stylexStyles3.rule36 : stylexStyles3.rule37,
      ]}
    >
      <span sx={stylexStyles3.rule38}>
        {metadata?.displayName || `Session ${sessionIndex}`}
      </span>
      <span sx={stylexStyles3.rule39}>/u/{sessionIndex}</span>
    </li>
  )
}

// UnderTheHood opens the devtools trees for SDK resources and state atoms.
function UnderTheHood({
  tileRef,
  onOpen,
}: {
  tileRef: TileRef
  onOpen: (id: InspectorId) => void
}) {
  const resourceCount = useTrackedResources().size
  const atomCount = useStateInspectorEntryMap().size

  return (
    <div sx={stylexStyles3.rule40}>
      <h2 sx={stylexStyles3.rule41}>Under the hood</h2>
      <DashboardButton
        ref={tileRef('resources')}
        icon={<LuLayers sx={stylexStyles.rule4} />}
        onClick={() => onOpen('resources')}
      >
        Resources
        <span sx={stylexStyles3.rule42}>{formatCount(resourceCount)}</span>
      </DashboardButton>
      <DashboardButton
        ref={tileRef('atoms')}
        icon={<LuBox sx={stylexStyles.rule4} />}
        onClick={() => onOpen('atoms')}
      >
        State atoms
        <span sx={stylexStyles3.rule42}>{formatCount(atomCount)}</span>
      </DashboardButton>
    </div>
  )
}

// InspectorNav returns to the tiles or hops straight to another inspector.
function InspectorNav({
  current,
  tones,
  onSelect,
  onBack,
}: {
  current: InspectorId
  tones: Record<SubsystemId, SystemTone>
  onSelect: (id: InspectorId) => void
  onBack: () => void
}) {
  return (
    <nav aria-label="System inspectors" sx={stylexStyles3.rule43}>
      <button type="button" onClick={onBack} sx={stylexStyles3.rule44}>
        <LuArrowLeft sx={stylexStyles.rule4} aria-hidden="true" />
        All systems
      </button>
      <span sx={stylexStyles3.rule45} aria-hidden="true" />
      {inspectorOrder.map((id) => {
        const tone = id in tones ? tones[id as SubsystemId] : null
        return (
          <button
            key={id}
            type="button"
            aria-current={id === current ? 'page' : undefined}
            onClick={() => onSelect(id)}
            sx={[
              stylexStyles3.rule46,
              id === current ? stylexStyles3.rule47 : stylexStyles3.rule48,
            ]}
          >
            {tone && (
              <span
                sx={[[stylexStyles5.rule2], [toneDotStyles[tone]]]}
                aria-hidden="true"
              />
            )}
            <span sx={[[false], [tone === 'error' && toneTextStyles.error]]}>
              {inspectorMeta[id].title}
            </span>
          </button>
        )
      })}
      <span sx={stylexStyles3.rule49}>
        <kbd sx={stylexStyles3.rule50}>Esc</kbd> to go back
      </span>
    </nav>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
