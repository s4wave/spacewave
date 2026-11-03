import type { IconType } from '@s4wave/web/images/icons.js'
import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
  },
  rule2: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
  },
  rule4: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface UseCaseCtaRowProps {
  children: ReactNode
}

// UseCaseCtaRow renders a centered row of call-to-action buttons.
export function UseCaseCtaRow({ children }: UseCaseCtaRowProps) {
  return <div sx={stylexStyles2.rule1}>{children}</div>
}

interface UseCaseCtaLinkProps {
  href: string
  icon: IconType
  variant?: 'primary' | 'default'
  children: ReactNode
}

// UseCaseCtaLink renders a single CTA anchor matching the landing page style.
export function UseCaseCtaLink({
  href,
  icon: Icon,
  variant = 'default',
  children,
}: UseCaseCtaLinkProps) {
  return (
    <a
      href={href}
      data-landing-primary-cta={variant === 'primary' || undefined}
      sx={[
        stylexStyles2.rule2,
        variant === 'primary' ? stylexStyles2.rule3 : stylexStyles2.rule4,
      ]}
    >
      <Icon sx={stylexStyles.rule1} />
      <span>{children}</span>
    </a>
  )
}
