import { useCallback } from 'react'
import { LuCreditCard } from '@s4wave/web/images/icons.js'

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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 1.5,
  },
  rule6: { color: themeColors['--color-green-500'] },
  rule7: { color: themeColors['--color-yellow-500'] },
  rule8: { color: themeColors['--color-destructive'] },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
      <div sx={stylexStyles3.rule1}>
        <LuCreditCard sx={stylexStyles.rule1} />
      </div>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <h4 sx={stylexStyles3.rule4}>Billing</h4>
          <span
            sx={[
              stylexStyles3.rule5,
              isStatusActive(status)
                ? stylexStyles3.rule6
                : isStatusPastDue(status)
                  ? stylexStyles3.rule7
                  : status === BillingStatus.BillingStatus_CANCELED
                    ? stylexStyles3.rule8
                    : stylexStyles3.rule9,
            ]}
          >
            {statusLabel(status)}
          </span>
        </div>
        <p sx={stylexStyles3.rule10}>
          {[intLabel, nextDate && `Next: ${nextDate}`]
            .filter(Boolean)
            .join(' \u00b7 ') || 'Manage your subscription'}
        </p>
      </div>
    </button>
  )
}
