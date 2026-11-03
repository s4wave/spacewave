import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    minWidth: 0,
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule4: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: themeColors['--color-muted-foreground'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule5: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-muted'],
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule6: {
    backgroundColor: themeColors['--color-foreground'],
    height: '100%',
    width: 'var(--context-used)',
  },
})

/** contextUsage clamps a negative used count and caps the meter at 100 percent. */
function contextUsage(used: number, limit: number) {
  const shown = Math.max(0, used)
  const percent =
    limit > 0 ? Math.min(100, Math.round((shown / limit) * 100)) : 0
  return { shown, percent }
}

/** Context renders used context as a meter. A zero limit is empty, not an error. */
function Context({
  used,
  limit,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    used: number
    limit: number
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const { shown, percent } = contextUsage(used, limit)

  return (
    <div
      data-slot="context"
      role="progressbar"
      aria-label="Context used"
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={percent}
      aria-valuetext={`${shown} of ${limit}`}
      {...stylexProps([stylexStyles.rule1, sx], props)}
    >
      <div sx={stylexStyles.rule2}>
        <span sx={stylexStyles.rule3}>{percent}%</span>
        <span sx={stylexStyles.rule4}>
          {shown} / {limit}
        </span>
      </div>
      <div sx={stylexStyles.rule5}>
        <div
          data-slot="context-meter"
          sx={stylexStyles.rule6}
          style={{ '--context-used': `${percent}%` }}
        />
      </div>
    </div>
  )
}

export { Context }
