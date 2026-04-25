import { useParams } from '@s4wave/web/router/router.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'

import { BillingStateProvider } from './BillingStateProvider.js'
import { BillingCancelPage } from './BillingCancelPage.js'

// BillingCancelRoute reads the baId route param and renders the cancel flow
// scoped to that BillingAccount.
export function BillingCancelRoute() {
  const params = useParams()
  const baId = params.baId ?? ''
  return (
    <BillingStateProvider billingAccountId={baId}>
      <SessionFrame>
        <BillingCancelPage />
      </SessionFrame>
    </BillingStateProvider>
  )
}
