import { useCallback, useEffect, useRef, useState } from 'react'

import type { BillingConsent } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { CLOUD_OFFER, formatStorageRate } from './pricing.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'grid' },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
  },
  rule4: { textDecorationLine: 'underline' },
  rule5: {
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: themeColors['--color-brand'],
    color: themeColors['--color-background'],
    opacity: { default: null, ':disabled': 0.5 },
    width: '100%',
  },
})

const dialogStyles = stylex.create({
  content: {
    maxHeight: themeLayout['--max-height-confirmation'],
    overflowY: 'auto',
  },
})

// BillingConsentForm binds the visible terms and spending choice to one action.
export function BillingConsentForm({
  onAccept,
  offer = CLOUD_OFFER,
  initialLimit = offer.defaultOverageLimitCents,
  spendingOnly = false,
  disabled = false,
}: {
  offer?: typeof CLOUD_OFFER
  onAccept: (consent: BillingConsent) => void
  initialLimit?: number
  spendingOnly?: boolean
  disabled?: boolean
}) {
  const [limit, setLimit] = useState(initialLimit)
  const monthlyPrice = offer.monthlyPriceCents / 100
  const label = spendingOnly
    ? 'Save spending limit'
    : `Subscribe for $${monthlyPrice}/month`
  return (
    <div sx={stylexStyles2.rule1}>
      <p sx={[nativeChildSpacing.s4, stylexStyles.rule1]}>
        Extra storage: {formatStorageRate(offer.storageMicrodollarsPerGibMonth)}
        , measured hourly. Reads and writes are not billed.
      </p>
      <label sx={[nativeChildSpacing.s4, stylexStyles.rule2]}>
        Monthly spending limit
        <select
          sx={stylexStyles.rule3}
          value={limit}
          disabled={disabled}
          onChange={(event) => setLimit(Number(event.target.value))}
        >
          {offer.overageLimitsCents.map((value) => (
            <option key={value} value={value}>
              {value === 0 ? 'Off' : `$${value / 100}/month`}
            </option>
          ))}
        </select>
      </label>
      {!spendingOnly && (
        <p sx={nativeChildSpacing.s4}>
          Your subscription renews automatically for ${monthlyPrice}/month
          before tax until you cancel online in Billing, effective at the paid
          period's end. By subscribing, you agree to the{' '}
          <a sx={stylexStyles.rule4} href="#/tos">
            Terms
          </a>
          , acknowledge the{' '}
          <a sx={stylexStyles.rule4} href="#/privacy">
            Privacy Policy
          </a>
          , and authorize extra storage up to the selected limit at the rate
          above.
        </p>
      )}
      {spendingOnly && (
        <p sx={nativeChildSpacing.s4}>
          By selecting “{label}”, you authorize extra storage up to the selected
          monthly limit at the rate above. Your base subscription stays $
          {monthlyPrice}/month. Lowering the limit preserves accrued charges.
        </p>
      )}
      <button
        type="button"
        sx={[nativeChildSpacing.s4, stylexStyles.rule5]}
        disabled={disabled}
        onClick={() =>
          onAccept({
            offerVersion: offer.version,
            policyVersion: offer.policyVersion,
            renewalAccepted: !spendingOnly,
            overageAccepted: limit > 0,
            overageLimitCents: limit,
          })
        }
      >
        {label}
      </button>
    </div>
  )
}

// useBillingConsent waits for the explicit action on the displayed offer.
// Dismissal and unmount cancel the pending request without granting agreement.
export function useBillingConsent() {
  // Keep one pending consent request and its displayed offer.
  const pending = useRef<((consent?: BillingConsent) => void) | null>(null)
  const [choice, setChoice] = useState<{
    limit: number
    spendingOnly: boolean
    offer: typeof CLOUD_OFFER
  } | null>(null)

  // Resolve the pending request and dismiss the offer together.
  const finish = useCallback((consent?: BillingConsent) => {
    const resolve = pending.current
    pending.current = null
    setChoice(null)
    resolve?.(consent)
  }, [])

  // Cancel an unanswered request when its owner unmounts.
  useEffect(
    () => () => {
      pending.current?.()
      pending.current = null
    },
    [],
  )

  // Replace any previous request with the new offer.
  const requestConsent = useCallback(
    (
      currentLimit = CLOUD_OFFER.defaultOverageLimitCents,
      spendingOnly = false,
      offer = CLOUD_OFFER,
    ) => {
      pending.current?.()
      setChoice({ limit: currentLimit, spendingOnly, offer })
      return new Promise<BillingConsent | undefined>((resolve) => {
        pending.current = resolve
      })
    },
    [],
  )

  // Render the current offer without granting consent on dismissal.
  const offer = choice?.offer ?? CLOUD_OFFER
  const consentDialog = (
    <Dialog
      open={choice !== null}
      onOpenChange={(value) => {
        if (!value) finish()
      }}
    >
      <DialogContent sx={dialogStyles.content}>
        <DialogTitle>
          {choice?.spendingOnly ? 'Spending limit' : 'Cloud monthly offer'}
        </DialogTitle>
        <DialogDescription>
          ${offer.monthlyPriceCents / 100}/month before tax. Includes{' '}
          {offer.storageBytes / 2 ** 30} GiB of encrypted cloud storage. Reads
          and writes are not billed.
        </DialogDescription>
        {choice && (
          <BillingConsentForm
            offer={choice.offer}
            initialLimit={choice.limit}
            spendingOnly={choice.spendingOnly}
            onAccept={finish}
          />
        )}
      </DialogContent>
    </Dialog>
  )
  return { requestConsent, consentDialog }
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})
