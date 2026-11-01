import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    minWidth: 0,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--leading-tight)',
    '--tw-leading': 'var(--leading-tight)',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    marginLeft: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

// Figure is one number in a FigureStrip.
export interface Figure {
  label: string
  value: string | number
  // unit follows the value in a smaller muted size, such as "of 4".
  unit?: string
  // detail is one line of context under the value.
  detail?: string
}

// FigureStripProps are the props for FigureStrip.
export interface FigureStripProps {
  figures: Figure[]
  className?: string
}

// FigureStrip shows a few figures side by side in one panel, each with a
// capital label, the value, and an optional line of context.
export function FigureStrip({ figures, className }: FigureStripProps) {
  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: divide-x */ cn(
          'border-foreground/6 bg-background-card/30 divide-foreground/6 grid auto-cols-fr grid-flow-col divide-x rounded-lg border',
          className,
        )
      }
    >
      {figures.map((figure) => (
        <div key={figure.label} sx={stylexStyles.rule1}>
          <div sx={stylexStyles.rule2}>{figure.label}</div>
          <div sx={stylexStyles.rule3}>
            {figure.value}
            {figure.unit && <span sx={stylexStyles.rule4}>{figure.unit}</span>}
          </div>
          {figure.detail && <div sx={stylexStyles.rule5}>{figure.detail}</div>}
        </div>
      ))}
    </div>
  )
}
