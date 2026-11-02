import { cn } from '@s4wave/web/style/utils.js'
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
  label,
  billing,
  onManage,
}: BillingAccountCardProps) {
  const status = billing.status
  const stColor = isStatusActive(status)
    ? 'text-green-500'
    : isStatusPastDue(status)
      ? 'text-yellow-500'
      : status === BillingStatus.BillingStatus_CANCELED
        ? 'text-destructive'
        : 'text-foreground-alt/50'

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
      sx={[stylexStyles.rule1, groupMarker]}
    >
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-xs font-medium">{label}</span>
          <span className={cn('micro-ten font-semibold', stColor)}>
            {statusLabel(status)}
          </span>
        </div>
        {detail && (
          <p className="text-foreground-alt/50 mt-0.5 text-xs">{detail}</p>
        )}
      </div>
      <span className="text-brand/60 shrink-0 self-center text-xs">Manage</span>
    </button>
  )
}
