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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'grid',
    gridAutoColumns: 'minmax(0, 1fr)',
    gridAutoFlow: 'column',
  },
  cell: {
    '--tw-divide-x-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    },
    borderInlineStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderInlineEndWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-x-reverse)))',
    },
    borderInlineStartWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-x-reverse))',
    },
    minWidth: 0,
  },
  label: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    textTransform: 'uppercase',
    userSelect: 'none',
    whiteSpace: 'nowrap',
  },
  value: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-leading': 'var(--leading-tight)',
    '--tw-numeric-spacing': 'tabular-nums',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-2xl)',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--leading-tight)',
    userSelect: 'none',
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  unit: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginLeft: themeLayout['--spacing'],
  },
  detail: {
    WebkitUserSelect: 'none',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    userSelect: 'none',
    whiteSpace: 'nowrap',
    marginTop: themeLayout['--spacing'],
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
