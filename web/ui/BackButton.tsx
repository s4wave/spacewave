import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type React from 'react'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    position: 'absolute',
    zIndex: 20,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: { WebkitUserSelect: 'none', userSelect: 'none' },
})

export interface BackButtonProps extends Omit<
  React.ButtonHTMLAttributes<HTMLButtonElement>,
  'className'
> {
  className?: never
  sx?: ComponentStyles
  floating?: boolean
}

// BackButton renders the shared arrow-left back action used across auth and
// full-page overlays.
export function BackButton({
  floating,
  sx,
  children,
  type = 'button',
  ...props
}: Omit<BackButtonProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <button
      type={type}
      {...stylexProps(
        [stylexStyles.rule1, !!floating && stylexStyles.rule2, sx],
        props,
      )}
    >
      <LuArrowLeft sx={stylexStyles.rule3} />
      <span sx={stylexStyles.rule4}>{children}</span>
    </button>
  )
}
