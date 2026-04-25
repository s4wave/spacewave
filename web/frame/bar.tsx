import React, { CSSProperties } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

// IBarProps are properties for a horizontal frame bar.
export interface IBarProps {
  // hidden hides the bar.
  hidden?: boolean
  // left are elements for the bottom bar left side.
  left?: React.ReactNode
  // leftStyle are additional styles for the left Flex box.
  leftStyle?: CSSProperties
  // right are elements for the bottom bar right side.
  right?: React.ReactNode
  // rightStyle are additional styles for the right Flex box.
  rightStyle?: CSSProperties
  // style are additional styles for the bar.
  style?: CSSProperties
  // className added to the root element, if applicable
  className?: string
  // hideTopBorder hides the top border.
  hideTopBorder?: boolean
}

export function Bar(props?: IBarProps) {
  if (props?.hidden) return null
  return (
    <div
      className={cn(
        'relative flex w-full flex-shrink-0 flex-row flex-nowrap gap-0',
        'bg-bar overflow-hidden',
        'text-center text-xs tabular-nums',
        'no-underline outline-0',
        'transition-colors duration-120',
        'min-height-bar h-bar text-nav text-nav-font',
        props?.className,
      )}
      style={props?.style}
    >
      <div
        className="flex flex-grow overflow-hidden text-left"
        style={props?.leftStyle}
      >
        {props?.left}
      </div>
      <div className="flex overflow-hidden" style={props?.rightStyle}>
        {props?.right}
      </div>
      {!props?.hideTopBorder ?
        <span className="after:bg-bar-border-top after:pointer-events-none after:absolute after:top-0 after:left-0 after:h-[1px] after:w-full after:content-['']" />
      : null}
    </div>
  )
}
