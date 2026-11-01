import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import React from 'react'
import { type VariantProps } from 'class-variance-authority'
import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  motion as themeMotion,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
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
  rule2: {
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
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    whiteSpace: { default: 'normal', '@media (min-width: 40rem)': 'nowrap' },
    color: {
      default: themeColors['--color-foreground'],
      ':hover': themeColors['--color-accent-foreground'],
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
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
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: themeColors['--color-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule4: {
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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      ':hover': themeColors['--color-accent'],
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    whiteSpace: { default: 'normal', '@media (min-width: 40rem)': 'nowrap' },
    color: {
      default: themeColors['--color-foreground'],
      ':hover': themeColors['--color-accent-foreground'],
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
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
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    whiteSpace: { default: 'normal', '@media (min-width: 40rem)': 'nowrap' },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const dashboardButtonVariants = (props?: {
  variant?: 'default' | 'active' | 'destructive' | 'primary' | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "variant" once, retaining explicit null instead of its default.
  const variant0Input = props?.['variant']
  const variant0 = variant0Input === undefined ? 'default' : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'default'
      ? stylexStyles.rule5
      : variant0 === 'active'
        ? stylexStyles.rule4
        : variant0 === 'destructive'
          ? stylexStyles.rule3
          : variant0 === 'primary'
            ? stylexStyles.rule2
            : stylexStyles.rule1,
    props?.class,
    props?.className,
  ] as const
}

/** DashboardButtonProps configures an icon action on a dashboard or panel. */
export interface DashboardButtonProps
  extends
    Omit<React.ComponentPropsWithRef<'button'>, 'className'>,
    Omit<VariantProps<typeof dashboardButtonVariants>, 'className'> {
  className?: never
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
      sx={[dashboardButtonVariants({ variant: variant }), sx]}
      {...props}
    >
      {icon}
      {children}
    </Button>
  )
}
