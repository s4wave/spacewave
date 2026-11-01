import * as stylex from '@stylexjs/stylex'

/** media shares the viewport and motion conditions used by component styles. */
export const media = stylex.defineConsts({
  sm: '@media (min-width: 40rem)',
  md: '@media (min-width: 48rem)',
  lg: '@media (min-width: 64rem)',
  xl: '@media (min-width: 80rem)',
  '2xl': '@media (min-width: 96rem)',
  reducedMotion: '@media (prefers-reduced-motion: reduce)',
  motion: '@media (prefers-reduced-motion: no-preference)',
  tall: '@media (min-height: 800px)',
  short: '@media (max-height: 580px)',
  'very-short': '@media (max-height: 470px)',
  'ultra-short': '@media (max-height: 390px)',
  narrow: '@media (max-width: 640px)',
  wide: '@media (min-width: 1920px)',
  ultrawide: '@media (min-width: 2560px)',
  'fine-pointer': '@media (pointer: fine)',
})
