import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import { badgeMarker, badgeIconStyles } from '../style/badge-icons.stylex.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  base: {
    display: 'inline-flex',
    width: 'fit-content',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    whiteSpace: 'nowrap',
    transitionProperty: 'color, box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
  },
  unselected: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  release: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  muted: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 65%, transparent)`,
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  success: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 25%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-success'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  brand: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  conflict: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  subtle: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  outline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':is(a)': {
          default: null,
          ':hover': themeColors['--color-accent-foreground'],
        },
      },
    },
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':is(a)': { default: null, ':hover': themeColors['--color-accent'] },
      },
    },
  },
  destructive: {
    borderColor: {
      default: '#0000',
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: themeColors['--color-destructive'],
      ':is(.dark *)': `color-mix(in oklab, ${themeColors['--color-destructive']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':is(a)': {
          default: null,
          ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 90%, transparent)`,
        },
      },
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-white'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':focus-visible': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  secondary: {
    borderColor: {
      default: '#0000',
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: themeColors['--color-secondary'],
      '@media (hover: hover)': {
        default: null,
        ':is(a)': {
          default: null,
          ':hover': `color-mix(in oklab, ${themeColors['--color-secondary']} 90%, transparent)`,
        },
      },
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-secondary-foreground'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
  default: {
    borderColor: {
      default: '#0000',
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: themeColors['--color-primary'],
      '@media (hover: hover)': {
        default: null,
        ':is(a)': {
          default: null,
          ':hover': `color-mix(in oklab, ${themeColors['--color-primary']} 90%, transparent)`,
        },
      },
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-primary-foreground'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
    },
  },
})

/** BadgeVariant selects the badge's semantic appearance. */
export type BadgeVariant =
  | 'default'
  | 'secondary'
  | 'destructive'
  | 'outline'
  | 'subtle'
  | 'conflict'
  | 'brand'
  | 'success'
  | 'muted'
  | 'release'

/** BadgeProps composes native span attributes with a finite appearance and caller styles. */
export type BadgeProps = Omit<React.ComponentProps<'span'>, 'className'> & {
  variant?: BadgeVariant | null
  asChild?: boolean
  sx?: ComponentStyles
  className?: never
}

/** badgeVariants returns the complete badge appearance for a native element. */
export function badgeVariants({
  variant,
  sx,
}: Pick<BadgeProps, 'variant' | 'sx'> = {}) {
  return [
    styles.base,
    variant === undefined
      ? styles.default
      : variant
        ? styles[variant]
        : styles.unselected,
    sx,
  ] as const
}

/** Badge renders a status label or lends its appearance to one child. */
export function Badge({ sx, variant, asChild = false, ...props }: BadgeProps) {
  const Comp = asChild ? Slot : 'span'
  return (
    <IconStylesProvider requiredSx={badgeIconStyles.icon}>
      <Comp
        data-slot="badge"
        {...stylexProps([badgeVariants({ variant, sx }), badgeMarker], props)}
      />
    </IconStylesProvider>
  )
}
