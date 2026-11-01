import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import React from 'react'
import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  motion as themeMotion,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  base: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': 0,
    },
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-menu-button'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    whiteSpace: { default: 'normal', '@media (min-width: 40rem)': 'nowrap' },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  plain: {},
  primary: {
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground'],
      ':hover': themeColors['--color-accent-foreground'],
    },
  },
  destructive: {
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: themeColors['--color-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  active: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      ':hover': themeColors['--color-accent'],
    },
    color: {
      default: themeColors['--color-foreground'],
      ':hover': themeColors['--color-accent-foreground'],
    },
  },
  default: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
})

const variantStyles = {
  default: styles.default,
  active: styles.active,
  destructive: styles.destructive,
  primary: styles.primary,
}

/** DashboardButtonProps configures an icon action on a dashboard or panel. */
export interface DashboardButtonProps extends Omit<
  React.ComponentPropsWithRef<'button'>,
  'className'
> {
  className?: never
  variant?: keyof typeof variantStyles | null
  sx?: ComponentStyles
  icon: React.ReactNode
  children?: React.ReactNode
}

/** DashboardButton renders a compact desktop action with a phone-sized touch target. */
export function DashboardButton({
  icon,
  children,
  sx,
  variant,
  ...props
}: Omit<DashboardButtonProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <Button
      variant="outline"
      size="sm"
      sx={[
        styles.base,
        variant === undefined
          ? variantStyles.default
          : variant
            ? variantStyles[variant]
            : styles.plain,
        sx,
      ]}
      {...props}
    >
      {icon}
      {children}
    </Button>
  )
}
