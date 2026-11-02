import { stylexProps } from '@s4wave/web/style/props.js'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'

const styles = stylex.create({
  border: {
    pointerEvents: 'none',
    position: 'absolute',
    inset: 0,
    width: '100%',
    height: '100%',
    borderRadius: 'inherit',
    backgroundImage: 'var(--shine-border-background)',
    backgroundSize: '300% 300%',
    mask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    WebkitMask:
      'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    maskPosition: '0px 0px, 0px 0px',
    WebkitMaskComposite: 'xor',
    maskComposite: 'exclude',
    padding: 'var(--border-width)',
    willChange: {
      default: 'background-position',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.35 },
    animationName: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'shine',
    },
    animationDuration: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'var(--duration)',
    },
    animationTimingFunction: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'linear',
    },
    animationIterationCount: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'infinite',
    },
  },
})

interface ShineBorderProps extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'className'
> {
  /**
   * Width of the border in pixels
   * @default 1
   */
  borderWidth?: number
  sx?: stylex.StyleXStyles
  className?: never
  /**
   * Duration of the animation in seconds
   * @default 14
   */
  duration?: number
  /**
   * Color of the border, can be a single color or an array of colors
   * @default "#000000"
   */
  shineColor?: string | string[]
}

/** ShineBorder draws an animated masked border around its positioned container. */
export function ShineBorder({
  borderWidth = 1,
  duration = 14,
  shineColor = '#000000',
  sx,
  style,
  ...props
}: ShineBorderProps) {
  return (
    <div
      {...stylexProps([styles.border, sx], {
        ...props,
        style: {
          '--border-width': `${borderWidth}px`,
          '--duration': `${duration}s`,
          '--shine-border-background': `radial-gradient(transparent,transparent, ${Array.isArray(shineColor) ? shineColor.join(',') : shineColor},transparent,transparent)`,
          ...style,
        },
      })}
    />
  )
}
