import { forgeControlStyles } from '@s4wave/web/style/forge-controls.stylex.js'
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
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
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
    <button
      type="button"
      onClick={onClick}
      sx={[sx ?? stylexStyles2.rule1, forgeControlStyles.required]}
    >
      {icon}
      <span sx={stylexStyles.rule1}>{children ?? label ?? objectKey}</span>
    </button>
  )
}
