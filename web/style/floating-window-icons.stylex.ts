import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from './tokens.stylex.js'

/** floatingWindowIconMarker limits icon dimensions to the header slot. */
export const floatingWindowIconMarker = stylex.defineMarker()
/** floatingWindowHeaderIconStyles preserves the header slot and its direct SVG dimensions. */
export const floatingWindowHeaderIconStyles = stylex.create({
  slot: {
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    height: 12,
    width: 12,
  },
  icon: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', floatingWindowIconMarker)]: '12px',
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', floatingWindowIconMarker)]: '12px',
    },
  },
})
