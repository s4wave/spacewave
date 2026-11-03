import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { LuCircleOff } from '@s4wave/web/images/icons.js'

import { chordDisplayTokens } from './keyboard-utils.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule2: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'inline-flex',
    minWidth: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: themeColors['--color-background-card-alt'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    boxShadow: 'inset 0 -1px #ffffff14',
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface KeycapProps {
  className?: never
  chord: string
  sx?: ComponentStyles
  muted?: boolean
}

export function Keycap({
  chord,
  sx,
  muted = false,
}: Omit<KeycapProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const tokens = chordDisplayTokens(chord)

  if (tokens.length === 0) {
    return (
      <span sx={[stylexStyles3.rule1, sx]}>
        <LuCircleOff sx={stylexStyles.rule1} />
        Unassigned
      </span>
    )
  }

  return (
    <span aria-label={chord} sx={[stylexStyles3.rule2, sx]}>
      {tokens.map((token) => (
        <kbd
          key={token}
          sx={[stylexStyles2.rule1, !!muted && stylexStyles2.rule2]}
        >
          {token}
        </kbd>
      ))}
    </span>
  )
}
