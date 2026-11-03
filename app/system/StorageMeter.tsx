import { formatBytes } from './format.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule3: { height: themeLayout['--spacing'] },
  rule4: { height: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule5: {
    borderRadius: '3.40282e38px',
    transition: 'width 500ms',
    transitionProperty: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    height: '100%',
    width: 'var(--progress-width)',
  },
  rule6: { backgroundColor: themeColors['--color-warning'] },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule8: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

// StorageMeter draws browser storage usage against its quota. It renders
// nothing when the browser did not report both numbers.
export function StorageMeter({
  usage,
  quota,
  compact = false,
}: {
  usage: number | null
  quota: number | null
  compact?: boolean
}) {
  if (usage == null || quota == null || quota <= 0) {
    return null
  }

  const ratio = Math.min(usage / quota, 1)
  const label = `${formatBytes(usage)} of ${formatBytes(quota)} browser quota`
  return (
    <div>
      <meter
        aria-label="Browser storage used"
        min={0}
        max={quota}
        value={usage}
        title={label}
        sx={stylexStyles.rule1}
      />
      <div
        aria-hidden
        sx={[
          stylexStyles.rule2,
          compact ? stylexStyles.rule3 : stylexStyles.rule4,
        ]}
      >
        <div
          sx={[
            stylexStyles.rule5,
            ratio >= 0.9 ? stylexStyles.rule6 : stylexStyles.rule7,
          ]}
          style={{ '--progress-width': `${Math.max(ratio * 100, 1)}%` }}
        />
      </div>
      {!compact && <p sx={stylexStyles.rule8}>{label}</p>}
    </div>
  )
}
