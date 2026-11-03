import {
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
} from '@s4wave/web/images/icons.js'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '@s4wave/app/session/SessionSyncStatusContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 48rem)': 'center' },
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 48rem)': 'row' },
    justifyContent: {
      default: null,
      '@media (min-width: 48rem)': 'space-between',
    },
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
    minWidth: 0,
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: { minWidth: 0 },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    width: {
      default: null,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 52)`,
    },
  },
  rule12: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule13: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.55rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule15: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    right: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

// SessionSyncStatusSummary renders a compact sync status card for SessionDetails.
export function SessionSyncStatusSummary({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const status = useSessionSyncStatus()
  return (
    <InfoCard sx={sx}>
      <div sx={stylexStyles3.rule1} data-testid="session-sync-status-summary">
        <div sx={stylexStyles3.rule2}>
          <div
            sx={[
              stylexStyles3.rule3,
              !!status.error && stylexStyles3.rule4,
              !!status.active && stylexStyles3.rule5,
              status.visualState === 'synced' && stylexStyles3.rule6,
              !!status.loading && stylexStyles3.rule7,
            ]}
          >
            <SessionSyncStatusSummaryIcon status={status} />
          </div>
          <div sx={stylexStyles3.rule8}>
            <div sx={stylexStyles3.rule9}>{status.summaryLabel}</div>
            <div sx={stylexStyles3.rule10}>{status.detailLabel}</div>
          </div>
        </div>
        <div sx={stylexStyles3.rule11}>
          <SummaryPill label="Up" value={status.uploadRateLabel} />
          <SummaryPill label="Down" value={status.downloadRateLabel} />
        </div>
      </div>
    </InfoCard>
  )
}

function SessionSyncStatusSummaryIcon({
  status,
}: {
  status: SessionSyncStatusView
}) {
  if (status.loading || status.active) {
    return <Spinner />
  }
  if (status.error) {
    return <LuCircleAlert sx={stylexStyles.rule1} aria-hidden="true" />
  }
  const Icon = status.local ? LuHardDrive : LuCloud
  return (
    <span sx={stylexStyles3.rule12} aria-hidden="true">
      <Icon sx={stylexStyles2.rule1} />
      <LuCircleCheck sx={stylexStyles.rule2} />
    </span>
  )
}

function SummaryPill({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles3.rule13}>
      <div sx={stylexStyles3.rule14}>{label}</div>
      <div sx={stylexStyles3.rule15}>{value}</div>
    </div>
  )
}
