import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../style/tokens.stylex.js'

import { colors as themeColors2 } from '../../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'absolute',
    insetBlock: 0,
    width: {
      default: '33.3333%',
      '@media (prefers-reduced-motion: reduce)': '100%',
    },
    animationName: 'progress-indeterminate',
    animationDuration: '1.4s',
    animationTimingFunction: 'ease-in-out',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors2['--color-brand'],
    left: { default: null, '@media (prefers-reduced-motion: reduce)': 0 },
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.7 },
  },
  rule2: {
    height: '100%',
    width: 'var(--progress-width)',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors2['--color-brand'],
    transitionProperty: {
      default: 'width',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    transitionTimingFunction: 'ease',
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    '--tw-ease': 'ease',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    flex: 1,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule3: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
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
export function ProgressBar({ value, indeterminate, rate }: ProgressBarProps) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        {indeterminate ? (
          <div sx={stylexStyles2.rule1} />
        ) : (
          <div
            sx={stylexStyles2.rule2}
            style={{ '--progress-width': `${pct}%` }}
          />
        )}
      </div>
      {rate ? (
        <span sx={stylexStyles.rule3}>{rate}</span>
      ) : !indeterminate ? (
        <span sx={stylexStyles.rule4}>{pct}%</span>
      ) : null}
    </div>
  )
}
