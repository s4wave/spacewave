import * as React from 'react'

// https://magicui.design/docs/components/shine-border

import { cn } from '@s4wave/web/style/utils.js'

interface ShineBorderProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * borderWidth is the width of the border in pixels.
   * @default 1
   */
  borderWidth?: number
  /**
   * duration is the duration of the animation in seconds.
   * @default 14
   */
  duration?: number
  /**
   * shineColor is the border color and can be a single color or an array
   * of colors.
   * @default "#000000"
   */
  shineColor?: string | string[]
}

/**
 * ShineBorder is an animated background border effect component with
 * configurable properties.
 */
export function ShineBorder({
  borderWidth = 1,
  duration = 14,
  shineColor = '#000000',
  className,
  style,
  ...props
}: ShineBorderProps) {
  return (
    <div
      style={{
        '--border-width': `${borderWidth}px`,
        '--duration': `${duration}s`,
        '--shine-border-background': `radial-gradient(transparent,transparent, ${
          Array.isArray(shineColor) ? shineColor.join(',') : shineColor
        },transparent,transparent)`,
        ...style,
      }}
      className={cn(
        'shine-border-mask motion-safe:animate-shine pointer-events-none absolute inset-0 size-full rounded-[inherit] will-change-[background-position] motion-reduce:opacity-35 motion-reduce:will-change-auto',
        className,
      )}
      {...props}
    />
  )
}
