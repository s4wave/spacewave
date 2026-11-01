import type { StyleXStyles } from '@stylexjs/stylex'
import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-ui-outline'],
    backgroundColor: themeColors['--color-background-secondary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  label: { color: themeColors['--color-foreground-alt'] },
  value: { color: themeColors['--color-foreground'] },
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
  // sx overrides the container styles.
  sx?: StyleXStyles
}

// StatsBar displays a horizontal bar of key-value statistics.
export function StatsBar({ stats, sx }: StatsBarProps) {
  if (stats.length === 0) return null

  return (
    <div sx={[styles.root, sx]}>
      {stats.map((stat) => (
        <span key={stat.label} sx={styles.label}>
          {stat.label}:{' '}
          <span sx={stat.valueSx ?? styles.value}>{stat.value}</span>
        </span>
      ))}
    </div>
  )
}
