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
    <div className="space-y-4 text-sm">
      <p className="text-foreground-alt text-xs">
        Extra storage: {formatStorageRate(offer.storageMicrodollarsPerGibMonth)}
        , measured hourly. Reads and writes are not billed.
      </p>
      <label className="grid gap-2">
        Monthly spending limit
        <select
          className="bg-background rounded border p-2"
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
        <p>
          Your subscription renews automatically for ${monthlyPrice}/month
          before tax until you cancel online in Billing, effective at the paid
          period's end. By subscribing, you agree to the{' '}
          <a className="underline" href="#/tos">
            Terms
          </a>
          , acknowledge the{' '}
          <a className="underline" href="#/privacy">
            Privacy Policy
          </a>
          , and authorize extra storage up to the selected limit at the rate
          above.
        </p>
      )}
      {spendingOnly && (
        <p>
          By selecting “{label}”, you authorize extra storage up to the selected
          monthly limit at the rate above. Your base subscription stays $
          {monthlyPrice}/month. Lowering the limit preserves accrued charges.
        </p>
      )}
      <button
        type="button"
        className="bg-brand text-background w-full rounded px-4 py-2 disabled:opacity-50"
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
