import { useCallback, type ReactNode } from 'react'

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

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
  className?: string
  children?: ReactNode
}

// ForgeEntityLink renders a clickable link that navigates to a forge entity.
// Uses SpaceContainerContext.navigateToObjects for in-space navigation.
export function ForgeEntityLink({
  objectKey,
  icon,
  label,
  className,
  children,
}: ForgeEntityLinkProps) {
  const container = SpaceContainerContext.useContextSafe()

  const onClick = useCallback(() => {
    container?.navigateToObjects([objectKey])
  }, [container, objectKey])

  if (!container) {
    return (
      <span
        className={
          /* TODO(stylex): dynamic class or forwarded className */ className
        }
      >
        {children ?? label ?? objectKey}
      </span>
    )
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={
        /* TODO(stylex): fallback class expression */ cn(
          className ??
            'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-2 rounded-lg border px-3 py-1.5 text-left transition-all duration-150',
        )
      }
    >
      {icon}
      <span sx={stylexStyles.rule1}>{children ?? label ?? objectKey}</span>
    </button>
  )
}
