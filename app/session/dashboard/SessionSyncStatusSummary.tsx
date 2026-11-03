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
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 48rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 48rem)': 'center' },
    justifyContent: {
      default: null,
      '@media (min-width: 48rem)': 'space-between',
    },
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
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
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    width: {
      default: null,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 52)`,
    },
  },
  rule12: {
    position: 'relative',
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule13: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule14: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * -1)`,
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-brand'],
  },
})

// SessionSyncStatusSummary renders a compact sync status card for SessionDetails.
export function SessionSyncStatusSummary() {
  const status = useSessionSyncStatus()
  return (
    <InfoCard>
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
