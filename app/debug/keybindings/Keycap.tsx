import { LuCircleOff } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import { chordDisplayTokens } from './keyboard-utils.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface KeycapProps {
  chord: string
  className?: string
  muted?: boolean
}

export function Keycap({ chord, className, muted = false }: KeycapProps) {
  const tokens = chordDisplayTokens(chord)

  if (tokens.length === 0) {
    return (
      <span
        className={cn(
          'text-foreground-alt/55 inline-flex items-center gap-1.5 text-xs',
          className,
        )}
      >
        <LuCircleOff sx={stylexStyles.rule1} />
        Unassigned
      </span>
    )
  }

  return (
    <span
      aria-label={chord}
      className={cn('inline-flex items-center gap-1', className)}
    >
      {tokens.map((token) => (
        <kbd
          key={token}
          className={cn(
            'border-foreground/15 bg-background-card-alt text-foreground-alt inline-flex min-w-6 items-center justify-center rounded border px-1.5 py-0.5 font-mono text-metadata leading-5 keycap-shadow',
            muted && 'text-foreground-alt/50 bg-background/60',
          )}
        >
          {token}
        </kbd>
      ))}
    </span>
  )
}
