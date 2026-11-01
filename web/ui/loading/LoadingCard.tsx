import type { StyleXStyles } from '@stylexjs/stylex'
import { LuCircleAlert, LuCircleCheck, LuCloud } from 'react-icons/lu'

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

const styles = stylex.create({
  root: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  iconBox: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  loading: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  active: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  synced: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-brand'],
  },
  error: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  progress: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  rates: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  actions: {
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  retryButton: {
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
  cancelButton: {
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
  header: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  body: { minWidth: 0, flex: 1 },
  title: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  detail: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  lastActivity: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  errorMessage: {
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
  errorIcon: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  syncedIcon: {
    position: 'relative',
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  cloudIcon: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  checkIcon: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * -1)`,
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-brand'],
  },
  ratePill: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rateLabel: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rateValue: {
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
  className?: never
  view: LoadingView
  sx?: StyleXStyles
}

// LoadingCardProgress renders the progress bar, or the transfer rates when no
// progress is known.
function LoadingCardProgress({ view }: { view: LoadingView }) {
  if (view.progress !== undefined || view.progressIndeterminate) {
    return (
      <div sx={styles.progress}>
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
    <div sx={styles.rates}>
      <RatePill label="Up" value={view.rate.up ?? '0 B/s'} />
      <RatePill label="Down" value={view.rate.down ?? '0 B/s'} />
    </div>
  )
}

// LoadingCardActions renders the retry and cancel buttons that the view offers.
function LoadingCardActions({ view }: { view: LoadingView }) {
  if (!view.onRetry && !view.onCancel) return null

  return (
    <div sx={styles.actions}>
      {view.onRetry ? (
        <button type="button" onClick={view.onRetry} sx={styles.retryButton}>
          {view.retryLabel ?? 'Retry'}
        </button>
      ) : null}
      {view.onCancel ? (
        <button type="button" onClick={view.onCancel} sx={styles.cancelButton}>
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
export function LoadingCard({ view, sx }: LoadingCardProps) {
  return (
    <div sx={[styles.root, sx]}>
      <div sx={styles.header}>
        <div
          sx={[
            styles.iconBox,
            view.state === 'loading'
              ? styles.loading
              : view.state === 'active'
                ? styles.active
                : view.state === 'synced'
                  ? styles.synced
                  : styles.error,
          ]}
        >
          <LoadingCardIcon state={view.state} />
        </div>
        <div sx={styles.body}>
          <div sx={styles.title}>{view.title}</div>
          {view.detail ? <div sx={styles.detail}>{view.detail}</div> : null}
          <LoadingCardProgress view={view} />
          {view.lastActivity ? (
            <div sx={styles.lastActivity}>{view.lastActivity}</div>
          ) : null}
          {view.error ? <div sx={styles.errorMessage}>{view.error}</div> : null}
          <LoadingCardActions view={view} />
        </div>
      </div>
    </div>
  )
}

function LoadingCardIcon({ state }: { state: LoadingState }) {
  if (state === 'error') {
    return (
      <LuCircleAlert {...stylex.props(styles.errorIcon)} aria-hidden="true" />
    )
  }
  if (state === 'synced') {
    return (
      <span sx={styles.syncedIcon} aria-hidden="true">
        <LuCloud {...stylex.props(styles.cloudIcon)} />
        <LuCircleCheck {...stylex.props(styles.checkIcon)} />
      </span>
    )
  }
  return <Spinner size="md" />
}

function RatePill({ label, value }: { label: string; value: string }) {
  return (
    <div sx={styles.ratePill}>
      <div sx={styles.rateLabel}>{label}</div>
      <div sx={styles.rateValue}>{value}</div>
    </div>
  )
}
