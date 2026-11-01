import { LuCircleAlert, LuCircleCheck, LuCloud } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { ProgressBar } from './ProgressBar.js'
import { Spinner } from './Spinner.js'
import type { LoadingState, LoadingView } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: '.65rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule5: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: '.65rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule6: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: { minWidth: 0, flex: 1 },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: '.65rem',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule12: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule13: {
    position: 'relative',
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule14: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule15: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * -1)`,
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-brand'],
  },
  rule16: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule17: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

interface LoadingCardProps {
  view: LoadingView
  className?: string
}

const tileTone: Record<LoadingState, string> = {
  loading: 'bg-foreground/5 text-foreground-alt',
  active: 'bg-brand/10 text-brand',
  synced: 'bg-foreground/5 text-brand',
  error: 'bg-destructive/10 text-destructive',
}

// LoadingCardProgress renders the progress bar, or the transfer rates when no
// progress is known.
function LoadingCardProgress({ view }: { view: LoadingView }) {
  if (view.progress !== undefined || view.progressIndeterminate) {
    return (
      <div sx={stylexStyles.rule1}>
        <ProgressBar
          value={view.progress === undefined ? undefined : view.progress * 100}
          indeterminate={view.progressIndeterminate}
          rate={view.rate?.down ?? view.rate?.up}
        />
      </div>
    )
  }
  if (!view.rate) return null

  return (
    <div sx={stylexStyles.rule2}>
      <RatePill label="Up" value={view.rate.up ?? '0 B/s'} />
      <RatePill label="Down" value={view.rate.down ?? '0 B/s'} />
    </div>
  )
}

// LoadingCardActions renders the retry and cancel buttons that the view offers.
function LoadingCardActions({ view }: { view: LoadingView }) {
  if (!view.onRetry && !view.onCancel) return null

  return (
    <div sx={stylexStyles.rule3}>
      {view.onRetry ? (
        <button type="button" onClick={view.onRetry} sx={stylexStyles.rule4}>
          {view.retryLabel ?? 'Retry'}
        </button>
      ) : null}
      {view.onCancel ? (
        <button type="button" onClick={view.onCancel} sx={stylexStyles.rule5}>
          {view.cancelLabel ?? 'Cancel'}
        </button>
      ) : null}
    </div>
  )
}

// LoadingCard is the primary loading surface. Glass card with an size-8 icon
// box, title, detail, optional progress, rate pills, last-activity footer,
// error box, and retry / cancel buttons. Four states: loading / active /
// synced / error.
export function LoadingCard({ view, className }: LoadingCardProps) {
  return (
    <div
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'border-foreground/6 bg-background-card/30 rounded-lg border p-3.5',
          className,
        )
      }
    >
      <div sx={stylexStyles.rule6}>
        <div
          className={
            /* TODO(stylex): dynamic class or forwarded className */ cn(
              'flex size-8 shrink-0 items-center justify-center rounded-md',
              tileTone[view.state],
            )
          }
        >
          <LoadingCardIcon state={view.state} />
        </div>
        <div sx={stylexStyles.rule7}>
          <div sx={stylexStyles.rule8}>{view.title}</div>
          {view.detail ? (
            <div sx={stylexStyles.rule9}>{view.detail}</div>
          ) : null}
          <LoadingCardProgress view={view} />
          {view.lastActivity ? (
            <div sx={stylexStyles.rule10}>{view.lastActivity}</div>
          ) : null}
          {view.error ? <div sx={stylexStyles.rule11}>{view.error}</div> : null}
          <LoadingCardActions view={view} />
        </div>
      </div>
    </div>
  )
}

function LoadingCardIcon({ state }: { state: LoadingState }) {
  if (state === 'error') {
    return (
      <LuCircleAlert
        {...stylex.props(stylexStyles.rule12)}
        aria-hidden="true"
      />
    )
  }
  if (state === 'synced') {
    return (
      <span sx={stylexStyles.rule13} aria-hidden="true">
        <LuCloud {...stylex.props(stylexStyles.rule14)} />
        <LuCircleCheck {...stylex.props(stylexStyles.rule15)} />
      </span>
    )
  }
  return <Spinner size="md" />
}

function RatePill({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles.rule16}>
      <div sx={stylexStyles.rule17}>{label}</div>
      <div sx={stylexStyles.rule18}>{value}</div>
    </div>
  )
}
