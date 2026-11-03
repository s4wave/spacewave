import { keyframeNames } from '../../style/tokens.stylex.js'
import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  indeterminate: {
    borderRadius: '3.40282e38px',
    insetBlock: 0,
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1.4s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['progress-indeterminate'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'ease-in-out',
    backgroundColor: themeColors['--color-brand'],
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.7 },
    position: 'absolute',
    left: { default: null, '@media (prefers-reduced-motion: reduce)': 0 },
    width: {
      default: '33.3333%',
      '@media (prefers-reduced-motion: reduce)': '100%',
    },
  },
  determinate: {
    '--tw-duration': '.2s',
    '--tw-ease': 'ease',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    transitionDuration: '.2s',
    transitionProperty: {
      default: 'width',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    transitionTimingFunction: 'ease',
    height: '100%',
    width: 'var(--progress-width)',
  },
  root: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  track: {
    borderRadius: '3.40282e38px',
    flex: 1,
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rate: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  percentage: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

// ProgressBarProps describes the inputs to ProgressBar.
interface ProgressBarProps {
  // value is the determinate progress value, 0..100.
  value?: number
  // indeterminate switches the bar to the sweeping animated variant.
  indeterminate?: boolean
  // rate is an optional right-side label shown in place of the percent.
  rate?: string
}

// ProgressBar renders a slim progress track with an optional right-side label.
// Determinate mode shows percent or a rate label; indeterminate mode runs the
// progress-indeterminate keyframe.
export function ProgressBar({
  sx,
  value,
  indeterminate,
  rate,
}: ProgressBarProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div sx={[styles.root, sx]}>
      <div sx={styles.track}>
        {indeterminate ? (
          <div sx={styles.indeterminate} />
        ) : (
          <div
            sx={styles.determinate}
            style={{ '--progress-width': `${pct}%` }}
          />
        )}
      </div>
      {rate ? (
        <span sx={styles.rate}>{rate}</span>
      ) : !indeterminate ? (
        <span sx={styles.percentage}>{pct}%</span>
      ) : null}
    </div>
  )
}
