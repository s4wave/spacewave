import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: { minWidth: 0 },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

// InspectorSection is one titled group of readings inside a subsystem
// inspector. The action slot holds controls that act on the section.
export function InspectorSection({
  title,
  description,
  action,
  children,
}: {
  title: string
  description?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <h3 sx={stylexStyles.rule4}>{title}</h3>
          {description && <p sx={stylexStyles.rule5}>{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}
