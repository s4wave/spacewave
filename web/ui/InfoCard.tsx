import React from 'react'
import * as stylex from '@stylexjs/stylex'

import { colors, fonts, layout, radii } from '../style/tokens.stylex.js'

const styles = stylex.create({
  card: {
    borderColor: `color-mix(in oklab, ${colors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${colors['--color-background-card']} 30%, transparent)`,
    minWidth: 0,
    borderRadius: radii['--radius-lg'],
    borderStyle: 'solid',
    borderWidth: 1,
    padding: `calc(${layout['--spacing']} * 3.5)`,
  },
  compact: { padding: `calc(${layout['--spacing']} * 2.5)` },
  heading: {
    color: colors['--color-foreground'],
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${layout['--spacing']} * 2)`,
    fontSize: fonts['--text-sm'],
    lineHeight: fonts['--text-sm--line-height'],
    userSelect: 'none',
    marginBottom: `calc(${layout['--spacing']} * 3)`,
  },
  compactHeading: { marginBottom: `calc(${layout['--spacing']} * 2)` },
  icon: {
    color: colors['--color-foreground'],
    display: 'flex',
    alignItems: 'center',
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
