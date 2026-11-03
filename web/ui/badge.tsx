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
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    overflow: 'hidden',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'inline-flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'color, box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    whiteSpace: 'nowrap',
    width: 'fit-content',
  },
  unselected: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
  },
  release: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    color: themeColors['--color-foreground-alt'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  muted: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 65%, transparent)`,
    fontWeight: 'var(--font-weight-normal)',
  },
  success: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 25%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
    color: themeColors['--color-success'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  brand: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    color: themeColors['--color-brand'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  conflict: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    color: themeColors['--color-destructive'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  subtle: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    fontWeight: 'var(--font-weight-normal)',
  },
  outline: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
    borderColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
      ':is([aria-invalid="true"])': {
        default: themeColors['--color-destructive'],
        ':focus-visible': themeColors['--color-destructive'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':is(a)': { default: null, ':hover': themeColors['--color-accent'] },
      },
    },
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
    fontWeight: themeFonts['--font-weight-medium'],
  },
  destructive: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':focus-visible': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
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
    color: themeColors['--color-white'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  secondary: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
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
    color: themeColors['--color-secondary-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  default: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
      ':is(.dark *)': {
        default: null,
        ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
      },
      ':is([aria-invalid="true"])': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    },
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
    color: themeColors['--color-primary-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
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
