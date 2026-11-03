import { useCallback, type ReactNode } from 'react'

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
})

interface ForgeEntityLinkProps {
  objectKey: string
  icon?: ReactNode
  label?: string
  sx?: stylex.StyleXStyles
  className?: never
  children?: ReactNode
}

// ForgeEntityLink renders a clickable link that navigates to a forge entity.
// Uses SpaceContainerContext.navigateToObjects for in-space navigation.
export function ForgeEntityLink({
  objectKey,
  icon,
  label,
  sx,
  children,
}: ForgeEntityLinkProps) {
  const container = SpaceContainerContext.useContextSafe()

  const onClick = useCallback(() => {
    container?.navigateToObjects([objectKey])
  }, [container, objectKey])

  if (!container) {
    return <span sx={sx}>{children ?? label ?? objectKey}</span>
  }

  return (
    <button type="button" onClick={onClick} sx={sx ?? stylexStyles2.rule1}>
      {icon}
      <span sx={stylexStyles.rule1}>{children ?? label ?? objectKey}</span>
    </button>
  )
}
