import React from 'react'
import * as stylex from '@stylexjs/stylex'

import { colors, fonts, layout, radii } from '../style/tokens.stylex.js'

const styles = stylex.create({
  card: {
    padding: `calc(${layout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${colors['--color-foreground']} 6%, transparent)`,
    borderRadius: radii['--radius-lg'],
    borderStyle: 'solid',
    borderWidth: 1,
    backgroundColor: `color-mix(in oklab, ${colors['--color-background-card']} 30%, transparent)`,
    minWidth: 0,
  },
  compact: { padding: `calc(${layout['--spacing']} * 2.5)` },
  heading: {
    gap: `calc(${layout['--spacing']} * 2)`,
    alignItems: 'center',
    color: colors['--color-foreground'],
    display: 'flex',
    fontSize: fonts['--text-sm'],
    lineHeight: fonts['--text-sm--line-height'],
    userSelect: 'none',
    marginBottom: `calc(${layout['--spacing']} * 3)`,
  },
  compactHeading: { marginBottom: `calc(${layout['--spacing']} * 2)` },
  icon: {
    alignItems: 'center',
    color: colors['--color-foreground'],
    display: 'flex',
    marginBottom: `calc(${layout['--spacing']} * 2)`,
  },
})

/** InfoCardProps controls the card's optional title, icon, spacing and outer styles. */
export interface InfoCardProps {
  icon?: React.ReactNode
  title?: string
  children: React.ReactNode
  compact?: boolean
  sx?: stylex.StyleXStyles
  className?: never
}

/** InfoCard displays related content with an optional heading and compact spacing. */
export function InfoCard({
  icon,
  title,
  children,
  compact = false,
  sx,
}: InfoCardProps) {
  return (
    <div sx={[styles.card, compact && styles.compact, sx]}>
      {title && (
        <h3 sx={[styles.heading, compact && styles.compactHeading]}>
          {icon}
          {title}
        </h3>
      )}
      {!title && icon && <div sx={styles.icon}>{icon}</div>}
      {children}
    </div>
  )
}
