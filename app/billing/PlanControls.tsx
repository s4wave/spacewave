import { useCallback } from 'react'
import { LuExternalLink, LuRefreshCw, LuX } from '@s4wave/web/images/icons.js'

import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { isStatusActive } from './billing-utils.js'
import { useBillingPortal } from './useBillingPortal.js'
import { usePlanReactivation } from './usePlanReactivation.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule6: {
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    cursor: 'pointer',
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// PlanControls renders the Plan section: the Stripe billing portal, and the
// cancel and reactivate actions when the viewer may manage the subscription.
export function PlanControls(
  props: {
    status?: BillingStatus
    cancelAt?: bigint | number
    showSelfService?: boolean
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  const navigate = useNavigate()
  const portal = useBillingPortal()

  const isActive = isStatusActive(props.status)
  const isCanceled = props.status === BillingStatus.BillingStatus_CANCELED
  const isCancelScheduled = isActive && !!props.cancelAt
  const cancelLabel = props.cancelAt
    ? new Date(Number(props.cancelAt)).toLocaleDateString()
    : null
  const reactivation = usePlanReactivation(
    !!props.showSelfService && isCanceled,
  )
  const { checkout } = reactivation

  const handleCancel = useCallback(() => {
    navigate({ path: './cancel' })
  }, [navigate])

  return (
    <div sx={props.sx}>
      {checkout.consentDialog}
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule1]}>Plan</div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule2]}>
        <DashboardButton
          icon={<LuExternalLink sx={stylexStyles.rule1} />}
          onClick={() => void portal.open()}
          disabled={!portal.ready || portal.opening}
        >
          {portal.opening ? 'Opening…' : 'Manage on Stripe'}
        </DashboardButton>
        {props.showSelfService && (
          <SubscriptionActions
            isActive={isActive}
            isCanceled={isCanceled}
            isCancelScheduled={isCancelScheduled}
            reactivating={reactivation.reactivating}
            reactivateBusy={reactivation.busy}
            onCancel={handleCancel}
            onReactivate={() => void reactivation.reactivate()}
          />
        )}
      </div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule3]}>
        Payment methods, invoices, and billing history are on Stripe.
        {isCancelScheduled &&
          cancelLabel &&
          ` Cancellation is scheduled for ${cancelLabel}. You keep full access until then.`}
      </div>
      <ReactivationProgress sx={nativeChildSpacing.s3} checkout={checkout} />
      {(portal.error || reactivation.error || checkout.error) && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule4]}>
          {portal.error || reactivation.error || checkout.error}
        </div>
      )}
    </div>
  )
}

// SubscriptionActions renders the cancel or reactivate button that fits the
// subscription's state.
function SubscriptionActions(props: {
  isActive: boolean
  isCanceled: boolean
  isCancelScheduled: boolean
  reactivating: boolean
  reactivateBusy: boolean
  onCancel: () => void
  onReactivate: () => void
}) {
  if (props.isCancelScheduled) {
    return (
      <DashboardButton
        icon={<LuRefreshCw sx={stylexStyles.rule1} />}
        onClick={props.onReactivate}
        disabled={props.reactivateBusy}
      >
        {props.reactivating ? 'Keeping subscription…' : 'Keep subscription'}
      </DashboardButton>
    )
  }
  if (props.isActive) {
    return (
      <DashboardButton
        icon={<LuX sx={stylexStyles.rule1} />}
        onClick={props.onCancel}
        variant="destructive"
      >
        Cancel subscription
      </DashboardButton>
    )
  }
  if (props.isCanceled) {
    return (
      <DashboardButton
        icon={<LuRefreshCw sx={stylexStyles.rule1} />}
        onClick={props.onReactivate}
        disabled={props.reactivateBusy}
      >
        {props.reactivating ? 'Reactivating…' : 'Reactivate subscription'}
      </DashboardButton>
    )
  }
  return null
}

// ReactivationProgress tells the viewer that Stripe checkout is pending and
// offers to reopen it.
function ReactivationProgress(
  props: {
    checkout: ReturnType<typeof useBillingAccountCheckout>
  } & { sx?: stylex.StyleXStyles; className?: never },
) {
  const { checkout } = props
  if (!checkout.polling) return null
  return (
    <div sx={[stylexStyles2.rule5, props.sx]}>
      Reactivation in progress. This page will update when Stripe confirms.
      {checkout.showRetry && (
        <button
          type="button"
          onClick={checkout.continueCheckout}
          sx={stylexStyles2.rule6}
        >
          Continue with Stripe
        </button>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
