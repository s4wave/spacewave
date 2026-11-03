import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'

const styles = stylex.create({
  border: {
    WebkitMask:
      'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    WebkitMaskComposite: 'xor',
    inset: 0,
    padding: 'var(--border-width)',
    borderRadius: 'inherit',
    mask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    animationDuration: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'var(--duration)',
    },
    animationIterationCount: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'infinite',
    },
    animationName: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': keyframeNames['shine'],
    },
    animationTimingFunction: {
      default: null,
      '@media (prefers-reduced-motion: no-preference)': 'linear',
    },
    backgroundImage: 'var(--shine-border-background)',
    backgroundSize: '300% 300%',
    maskComposite: 'exclude',
    maskPosition: '0px 0px, 0px 0px',
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.35 },
    pointerEvents: 'none',
    position: 'absolute',
    willChange: {
      default: 'background-position',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    height: '100%',
    width: '100%',
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
