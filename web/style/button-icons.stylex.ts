import * as stylex from '@stylexjs/stylex'

/** buttonMarker limits icon rules to this button's DOM descendants. */
export const buttonMarker = stylex.defineMarker()
/** buttonIconStyles preserves the button's SVG rules at each inherited icon. */
export const buttonIconStyles = stylex.create({
  icon: {
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonMarker)]: '16px',
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonMarker)]: '16px',
    },
    flexShrink: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonMarker)]: 0,
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonMarker)]: 'none',
    },
  },
})
