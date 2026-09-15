import React, { CSSProperties } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// IBarProps are properties for a horizontal frame bar.
export interface IBarProps {
  // hidden hides the bar.
  hidden?: boolean
  // left are elements for the bottom bar left side.
  left?: React.ReactNode
  // right are elements for the bottom bar right side.
  right?: React.ReactNode
  // style are additional styles for the bar.
  style?: CSSProperties
  // className added to the root element, if applicable
  className?: string
  // hideTopBorder hides the top border.
  hideTopBorder?: boolean
}

export function Bar({
  hidden,
  className,
  style,
  left,
  right,
  hideTopBorder,
}: IBarProps = {}) {
  if (hidden) return null
  return (
    <div
      className={cn(
        'relative flex w-full flex-shrink-0 flex-row flex-nowrap gap-0',
        'bg-bar overflow-hidden',
        'text-center text-xs tabular-nums',
        'no-underline outline-0',
        'transition-colors duration-120',
        'min-h-bar h-bar text-bar-font',
        className,
      )}
      style={style}
    >
      <div className="flex min-w-0 flex-grow overflow-hidden text-left">
        {left}
      </div>
      <div className="flex shrink-0 overflow-hidden">{right}</div>
      {!hideTopBorder ? (
        <span className="after:bg-bar-border-top after:content-blank after:pointer-events-none after:absolute after:top-0 after:left-0 after:h-0.25 after:w-full" />
      ) : null}
    </div>
  )
}
