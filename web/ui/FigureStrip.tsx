import type { StyleXStyles } from '@stylexjs/stylex'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    display: 'grid',
    gridAutoFlow: 'column',
    gridAutoColumns: 'minmax(0, 1fr)',
    borderRadius: themeRadii['--radius-lg'],
    borderWidth: '1px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  cell: {
    '--tw-divide-x-reverse': { default: null, ':not(:last-child)': 0 },
    borderInlineStartWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-x-reverse))',
    },
    borderInlineEndWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-x-reverse)))',
    },
    borderInlineStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderColor: {
      default: null,
      ':not(:last-child)': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    },
    minWidth: 0,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  label: {
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
  value: {
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
  unit: {
    marginLeft: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: themeColors['--color-foreground-alt'],
  },
  detail: {
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
  sx?: StyleXStyles
  className?: never
}

// FigureStrip shows a few figures side by side in one panel, each with a
// capital label, the value, and an optional line of context.
export function FigureStrip({ figures, sx }: FigureStripProps) {
  return (
    <div sx={[styles.root, sx]}>
      {figures.map((figure) => (
        <div key={figure.label} sx={styles.cell}>
          <div sx={styles.label}>{figure.label}</div>
          <div sx={styles.value}>
            {figure.value}
            {figure.unit && <span sx={styles.unit}>{figure.unit}</span>}
          </div>
          {figure.detail && <div sx={styles.detail}>{figure.detail}</div>}
        </div>
      ))}
    </div>
  )
}
