import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type React from 'react'
import { LuChevronDown } from '@s4wave/web/images/icons.js'

import { Button } from './button.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

import {
  layout as themeLayout2,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { gap: `calc(${themeLayout2['--spacing']} * 1.5)` },
  rule2: {
    height: 'auto',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout2['--spacing']} * 2.5)`,
    paddingBlock: themeLayout2['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
  },
  rule3: {
    height: 'auto',
    paddingInline: 0,
    paddingBlock: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': '#0000' },
    },
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface DropdownTriggerButtonProps extends Omit<
  React.ButtonHTMLAttributes<HTMLButtonElement>,
  'className'
> {
  className?: never
  sx?: ComponentStyles
  icon?: React.ReactNode
  triggerStyle?: 'outline' | 'ghost'
  showChevron?: boolean
}

// DropdownTriggerButton renders the shared button shell for dropdown triggers.
export function DropdownTriggerButton({
  icon,
  triggerStyle = 'outline',
  showChevron = true,
  sx,
  children,
  type = 'button',
  ...props
}: Omit<DropdownTriggerButtonProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <Button
      type={type}
      variant={triggerStyle === 'ghost' ? 'ghost' : 'outline'}
      size="sm"
      sx={[
        stylexStyles2.rule1,
        triggerStyle === 'outline' ? stylexStyles2.rule2 : stylexStyles2.rule3,
        sx,
      ]}
      {...props}
    >
      {icon}
      <span>{children}</span>
      {showChevron && <LuChevronDown {...stylex.props(stylexStyles.rule1)} />}
    </Button>
  )
}
