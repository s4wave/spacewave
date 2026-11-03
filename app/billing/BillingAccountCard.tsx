import {
  BillingStatus,
  type BillingAccountInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  statusLabel,
  intervalLabel,
  isStatusActive,
  isStatusPastDue,
} from './billing-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule5: { color: themeColors['--color-green-500'] },
  rule6: { color: themeColors['--color-yellow-500'] },
  rule7: { color: themeColors['--color-destructive'] },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    flexShrink: 0,
    alignSelf: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

export interface BillingAccountCardProps {
  label: string
  billing: BillingAccountInfo
  onManage: () => void
}

// BillingAccountCard renders a compact billing account card with status badge,
// interval, next date, and manage action.
export function BillingAccountCard({
  sx,
  label,
  billing,
  onManage,
}: BillingAccountCardProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const status = billing.status

  const intLabel = intervalLabel(billing.billingInterval)
  const nextDate = billing.currentPeriodEnd
    ? new Date(Number(billing.currentPeriodEnd)).toLocaleDateString()
    : ''

  const detail = [intLabel, nextDate && `Next: ${nextDate}`]
    .filter(Boolean)
    .join(' \u00b7 ')

  return (
    <button
      type="button"
      onClick={onManage}
      sx={[[stylexStyles.rule1, groupMarker], sx]}
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <span sx={stylexStyles2.rule3}>{label}</span>
          <span
            sx={[
              stylexStyles2.rule4,
              isStatusActive(status)
                ? stylexStyles2.rule5
                : isStatusPastDue(status)
                  ? stylexStyles2.rule6
                  : status === BillingStatus.BillingStatus_CANCELED
                    ? stylexStyles2.rule7
                    : stylexStyles2.rule8,
            ]}
          >
            {statusLabel(status)}
          </span>
        </div>
        {detail && <p sx={stylexStyles2.rule9}>{detail}</p>}
      </div>
      <span sx={stylexStyles2.rule10}>Manage</span>
    </button>
  )
}
