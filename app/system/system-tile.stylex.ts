import * as stylex from '@stylexjs/stylex'

// tileIconMarker limits the title glyph requirement to its direct SVG owner.
export const tileIconMarker = stylex.defineMarker()

// tileSurfaceStyles keeps the tile focus ring and title icon at their rendered owners.
export const tileSurfaceStyles = stylex.create({
  focus: {
    '--tw-ring-color': {
      default: null,
      ':has(button:focus-visible)':
        'color-mix(in oklab, var(--color-brand) 50%, transparent)',
    },
    '--tw-ring-shadow': {
      default: null,
      ':has(button:focus-visible)':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':has(button:focus-visible)':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
  },
  iconWrap: { flexShrink: 0 },
  icon: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', tileIconMarker)]: 14,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', tileIconMarker)]: 14,
    },
  },
})
