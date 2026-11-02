import { useCallback } from 'react'
import { LuCreditCard } from '@s4wave/web/images/icons.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import {
  statusLabel,
  intervalLabel,
  isStatusActive,
  isStatusPastDue,
} from './billing-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
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
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

// BillingSummary is a compact billing card for the session details overlay.
export function BillingSummary() {
  const billingState = useBillingStateContext()
  const navigateSession = useSessionNavigate()
  const billing = billingState.response?.billingAccount
  const baId = billing?.id
  const handleManage = useCallback(() => {
    if (baId) {
      navigateSession({ path: `billing/${baId}` })
      return
    }
    navigateSession({ path: 'billing' })
  }, [navigateSession, baId])

  if (!billing) return null

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

  return (
    <button
      type="button"
      onClick={handleManage}
      sx={[stylexStyles2.rule1, groupMarker2]}
    >
      <div className="bg-foreground/10 group-hover:bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
        <LuCreditCard sx={stylexStyles.rule1} />
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2">
          <h4 className="text-foreground text-xs font-medium select-none">
            Billing
          </h4>
          <span className={cn('micro-ten font-semibold', stColor)}>
            {statusLabel(status)}
          </span>
        </div>
        <p className="text-foreground-alt text-xs select-none">
          {[intLabel, nextDate && `Next: ${nextDate}`]
            .filter(Boolean)
            .join(' \u00b7 ') || 'Manage your subscription'}
        </p>
      </div>
    </button>
  )
}
