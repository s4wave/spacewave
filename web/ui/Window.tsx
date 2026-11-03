import { stylexProps } from '@s4wave/web/style/props.js'
import React, { type ReactNode, useState } from 'react'
import * as stylex from '@stylexjs/stylex'
import { colors, radii, utilityState } from '../style/tokens.stylex.js'

const styles = stylex.create({
  window: {
    dynamicRangeLimit: {
      default: null,
      '@media (dynamic-range: high)': { default: null, ':hover': 'no-limit' },
    },
    borderRadius: radii['--radius-lg'],
    borderStyle: utilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    boxShadow: {
      default: null,
      '@media (dynamic-range: high)': {
        default: null,
        ':hover': '0 0 6px 1px var(--hdr-glow-active-shadow)',
      },
    },
    transitionDuration: {
      default: null,
      '@media (dynamic-range: high)': '.15s',
    },
    transitionProperty: {
      default: null,
      '@media (dynamic-range: high)': 'box-shadow, border-color',
    },
    transitionTimingFunction: {
      default: null,
      '@media (dynamic-range: high)': 'ease',
    },
  },
  idle: { borderColor: colors['--color-window-border'] },
  hovered: { borderColor: colors['--color-ui-outline-active'] },
})

interface WindowProps {
  children: ReactNode
  sx?: stylex.StyleXStyles
  className?: never
  style?: React.CSSProperties
  'data-area-id'?: string
}

// Window frames its content and highlights its border while hovered.
export function Window({
  children,
  sx,
  style,
  'data-area-id': dataAreaId,
}: WindowProps) {
  const [isHovered, setIsHovered] = useState(false)

  return (
    <div
      {...stylexProps(
        [styles.window, isHovered ? styles.hovered : styles.idle, sx],
        { style },
      )}
      data-area-id={dataAreaId}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {children}
    </div>
  )
}
