import { useState } from 'react'

import type { BillingUsageInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

import { useBillingConsent } from '../provider/spacewave/useBillingConsent.js'
import { CLOUD_OFFER } from '../provider/spacewave/pricing.js'
import { useBillingStateContext } from './BillingStateProvider.js'

// useSpendingLimit changes the payer's extra storage spending limit. It
// describes the payer's accepted offer for the consent dialog, which the
// caller renders as consentDialog.
export function useSpendingLimit(usage: BillingUsageInfo) {
  const billingState = useBillingStateContext()
  const session = SessionContext.useContext().value
  const { requestConsent, consentDialog } = useBillingConsent()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Describe the payer's accepted offer for the spending limit consent.
  const offer = {
    ...CLOUD_OFFER,
    version: usage.offerVersion ?? CLOUD_OFFER.version,
    policyVersion: usage.policyVersion ?? CLOUD_OFFER.policyVersion,
    monthlyPriceCents: usage.monthlyPriceCents ?? CLOUD_OFFER.monthlyPriceCents,
    storageMicrodollarsPerGibMonth:
      usage.storageMicrodollarsPerGibMonth ??
      CLOUD_OFFER.storageMicrodollarsPerGibMonth,
    storageBytes: usage.storageBaselineBytes ?? 1,
  }

  // changeLimit asks for consent to a new spending limit and stores it.
  async function changeLimit() {
    if (!session || saving) return
    const consent = await requestConsent(
      usage.overageLimitCents ?? 0,
      true,
      offer,
    )
    if (!consent) return
    setSaving(true)
    setError(null)
    try {
      await session.spacewave.setBillingSpendingLimit(
        consent,
        billingState.billingAccountId,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  return {
    offer,
    saving,
    error,
    changeLimit,
    consentDialog,
    canChange: !!session,
  }
}
