import * as stylex from '@stylexjs/stylex'

/** badgeMarker limits badge icon rules to its direct DOM children. */
export const badgeMarker = stylex.defineMarker()
/** badgeIconStyles preserves badge SVG dimensions and pointer handling at the icon. */
export const badgeIconStyles = stylex.create({
  icon: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', badgeMarker)]: '12px',
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', badgeMarker)]: '12px',
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*) >', badgeMarker)]: 'none',
    },
  },
})
