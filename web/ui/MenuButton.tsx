import { type ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  motion as themeMotion,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-menu-button'],
    paddingInline: '4px',
    fontSize: themeLayout['--text-topbar-menu'],
    whiteSpace: 'nowrap',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    textShadow: themeLayout['--text-shadow-glow'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
  },
})

interface MenuButtonProps {
  children: ReactNode
  onClick?: () => void
}

export function MenuButton({
  sx,
  children,
  onClick,
}: MenuButtonProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <button type="button" onClick={onClick} sx={[[stylexStyles.rule1], sx]}>
      {children}
    </button>
  )
}
