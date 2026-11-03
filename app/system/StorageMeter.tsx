import { formatBytes } from './format.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule2: {
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule3: { height: themeLayout['--spacing'] },
  rule4: { height: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule5: {
    width: 'var(--progress-width)',
    transition: 'width 500ms',
    height: '100%',
    borderRadius: '3.40282e38px',
    transitionProperty: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
  },
  rule6: { backgroundColor: themeColors['--color-warning'] },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule8: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
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
