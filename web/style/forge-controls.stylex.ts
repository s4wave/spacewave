import * as stylex from '@stylexjs/stylex'

/** forgeViewerMarker scopes control sizing to controls inside a Forge viewer. */
export const forgeViewerMarker = stylex.defineMarker()

/** forgeControlStyles applies the viewer's pointer and touch floors at each native button. */
export const forgeControlStyles = stylex.create({
  required: {
    minHeight: {
      default: null,
      [stylex.when.ancestor(':is(*)', forgeViewerMarker)]: {
        default: null,
        ':is(button)': { default: '24px', '@media (pointer: coarse)': '44px' },
      },
    },
    minWidth: {
      default: null,
      [stylex.when.ancestor(':is(*)', forgeViewerMarker)]: {
        default: null,
        ':is(button)': { default: '24px', '@media (pointer: coarse)': '44px' },
      },
    },
  },
})
