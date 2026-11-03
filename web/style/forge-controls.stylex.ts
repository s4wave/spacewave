import * as stylex from '@stylexjs/stylex'

/** forgeViewerMarker scopes touch sizing to controls inside a Forge viewer. */
export const forgeViewerMarker = stylex.defineMarker()

/** forgeControlStyles applies the viewer's touch floor at each native button. */
export const forgeControlStyles = stylex.create({
  required: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': {
        default: null,
        [stylex.when.ancestor(':is(*)', forgeViewerMarker)]: {
          default: null,
          ':is(button)': '44px',
        },
      },
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': {
        default: null,
        [stylex.when.ancestor(':is(*)', forgeViewerMarker)]: {
          default: null,
          ':is(button)': '44px',
        },
      },
    },
  },
})
