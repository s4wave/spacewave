import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { StyleXStyles } from '@stylexjs/stylex'
import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from '../style/tokens.stylex.js'

import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors2,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors2['--color-ui-outline'],
    backgroundColor: themeColors2['--color-background-secondary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: { color: themeColors['--color-foreground-alt'] },
  rule2: { color: themeColors['--color-foreground'] },
})

// StatItem represents a single statistic to display.
export interface StatItem {
  // label is the stat label text.
  label: string
  // value is the stat value (can be string, number, or ReactNode).
  value: ReactNode
  // valueSx styles the value; absent styles use the standard foreground.
  valueSx?: StyleXStyles
  valueClassName?: never
}

// StatsBarProps are the props for the StatsBar component.
export interface StatsBarProps {
  className?: never
  // stats is the array of statistics to display.
  stats: StatItem[]
  // className is an optional CSS class for the container.
  sx?: ComponentStyles
}

// StatsBar displays a horizontal bar of key-value statistics.
export function StatsBar({
  stats,
  sx,
}: Omit<StatsBarProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  if (stats.length === 0) return null

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      {stats.map((stat) => (
        <span key={stat.label} sx={stylexStyles.rule1}>
          {stat.label}:{' '}
          <span sx={stat.valueSx ?? stylexStyles.rule2}>{stat.value}</span>
        </span>
      ))}
    </div>
  )
}
