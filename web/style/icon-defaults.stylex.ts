import * as stylex from '@stylexjs/stylex'
import { colors } from './tokens.stylex.js'

/** menuIconMarker gates the menu's default SVG dimensions to its DOM descendants. */
export const menuIconMarker = stylex.defineMarker()
/** menuMutedIconMarker gates the neutral item color without coloring checkbox indicators. */
export const menuMutedIconMarker = stylex.defineMarker()
/** commandIconMarker gates command item defaults to its rendered descendants. */
export const commandIconMarker = stylex.defineMarker()
/** buttonGroupIconMarker gates the button group label's SVG defaults. */
export const buttonGroupIconMarker = stylex.defineMarker()
/** traceIconMarker gates trace indicator dimensions. */
export const traceIconMarker = stylex.defineMarker()
/** codeTitleIconMarker gates the code title's compact SVG dimensions. */
export const codeTitleIconMarker = stylex.defineMarker()

/** iconDefaults composes DOM-scoped defaults before an icon's explicit caller styles. */
export const iconDefaults = stylex.create({
  menu: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 16,
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 16,
    },
  },
  menuMuted: {
    color: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuMutedIconMarker)]:
        colors['--color-muted-foreground'],
    },
  },
  command: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 16,
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 16,
    },
    color: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]:
        colors['--color-foreground-alt'],
    },
  },
  buttonGroup: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonGroupIconMarker)]: 16,
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonGroupIconMarker)]: 16,
    },
  },
  trace: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceIconMarker)]: 16,
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceIconMarker)]: 16,
    },
  },
  codeTitle: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 14,
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 14,
    },
  },
})

/** iconRequired retains menu interaction rules independently of explicit caller defaults. */
export const iconRequired = stylex.create({
  menu: {
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 'none',
    },
    flexShrink: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 0,
    },
  },
  menuDestructive: {
    color: {
      default: null,
      [stylex.when.ancestor(
        ':is([data-variant="destructive"]) >',
        menuIconMarker,
      )]: `${colors['--color-destructive']} !important`,
    },
  },
})
