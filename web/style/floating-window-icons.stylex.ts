import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from './tokens.stylex.js'

/** floatingWindowIconMarker limits icon dimensions to the header slot. */
export const floatingWindowIconMarker = stylex.defineMarker()
/** floatingWindowHeaderIconStyles preserves the header slot and its direct SVG dimensions. */
export const floatingWindowHeaderIconStyles = stylex.create({
  slot: {
    display: 'flex',
    width: 12,
    height: 12,
    alignItems: 'center',
    justifyContent: 'center',
    color: themeColors['--color-brand'],
  },
  icon: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', floatingWindowIconMarker)]: '12px',
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', floatingWindowIconMarker)]: '12px',
    },
  },
})
