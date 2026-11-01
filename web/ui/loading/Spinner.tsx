import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { LuLoader } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  animation: {
    animationName: {
      default: 'spin',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: '1s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: 'linear',
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: 'infinite',
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: 'normal',
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: 'running',
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: '0s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: 'none',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: 'auto',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
  },
  small: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  medium: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  large: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  extraLarge: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  foreground: { color: themeColors['--color-foreground'] },
  muted: { color: themeColors['--color-foreground-alt'] },
  subtle: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  brand: { color: themeColors['--color-brand'] },
  destructive: { color: themeColors['--color-destructive'] },
  success: { color: themeColors['--color-success'] },
})

// SpinnerSize controls the rendered dimensions of Spinner.
export type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

export type SpinnerVariant =
  | 'default'
  | 'foreground'
  | 'muted'
  | 'subtle'
  | 'brand'
  | 'destructive'
  | 'success'

const sizes = {
  sm: styles.small,
  md: styles.medium,
  lg: styles.large,
  xl: styles.extraLarge,
}

const tones = {
  default: undefined,
  foreground: styles.foreground,
  muted: styles.muted,
  subtle: styles.subtle,
  brand: styles.brand,
  destructive: styles.destructive,
  success: styles.success,
}

interface SpinnerProps {
  className?: never
  size?: SpinnerSize
  variant?: SpinnerVariant
  sx?: ComponentStyles
}

// Spinner renders the atomic animated loading indicator used across the app.
// Inherits text color from the parent so container state colors apply.
export function Spinner({ size = 'md', variant, sx }: SpinnerProps) {
  return (
    <LuLoader
      {...stylex.props(
        styles.animation,
        sizes[size],
        tones[variant ?? 'default'],
        sx,
      )}
      aria-hidden="true"
    />
  )
}
