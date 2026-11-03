import * as stylex from '@stylexjs/stylex'
import { layout } from './tokens.stylex.js'

/** viewerBarMarker scopes the existing phone workbench layout to its bar. */
export const viewerBarMarker = stylex.defineMarker()

/** viewerBarStyles keeps touch sizing at the bar and the actual control owners. */
export const viewerBarStyles = stylex.create({
  root: {
    paddingInline: layout['--spacing'],
    height: {
      default: layout['--spacing-bar'],
      '@media (pointer: coarse)': 'calc(44px + env(safe-area-inset-bottom))',
    },
    minHeight: {
      default: layout['--spacing-bar'],
      '@media (pointer: coarse)': 'calc(44px + env(safe-area-inset-bottom))',
    },
    paddingBottom: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        'env(safe-area-inset-bottom)',
    },
  },
  right: {
    maxWidth: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        {
          default: null,
          [stylex.when.ancestor(':is(*) >', viewerBarMarker)]: '55%',
        },
    },
  },
  required: {
    flexShrink: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        {
          default: null,
          [stylex.when.ancestor(
            ':is(*) > div:first-of-type >',
            viewerBarMarker,
          )]: { default: null, ':last-child': 1 },
        },
    },
    minHeight: {
      default: null,
      [stylex.when.ancestor(':is(*)', viewerBarMarker)]: {
        default: null,
        ':is(button, [role="button"])': {
          default: '24px',
          '@media (pointer: coarse)': '44px',
        },
      },
    },
    minWidth: {
      default: null,
      [stylex.when.ancestor(':is(*)', viewerBarMarker)]: {
        default: null,
        ':is(button, [role="button"])': {
          default: '24px',
          '@media (pointer: coarse)': '44px',
        },
      },
    },
  },
})
