import React, { CSSProperties } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flexGrow: 1,
    overflow: 'hidden',
    textAlign: 'left',
  },
  rule2: { display: 'flex', flexShrink: 0, overflow: 'hidden' },
  rule3: {
    '::after': {
      content: '""',
      pointerEvents: 'none',
      position: 'absolute',
      top: 0,
      left: 0,
      height: `calc(${themeLayout['--spacing']} * .25)`,
      width: '100%',
      backgroundColor: themeColors['--color-bar-border-top'],
    },
  },
})

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
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'relative flex w-full flex-shrink-0 flex-row flex-nowrap gap-0',
          'bg-bar overflow-hidden',
          'text-center text-xs tabular-nums',
          'no-underline outline-0',
          'transition-colors duration-120',
          'min-h-bar h-bar text-bar-font',
          className,
        )
      }
      style={style}
    >
      <div sx={stylexStyles.rule1}>{left}</div>
      <div sx={stylexStyles.rule2}>{right}</div>
      {!hideTopBorder ? <span sx={stylexStyles.rule3} /> : null}
    </div>
  )
}
