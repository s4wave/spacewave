import { useParams } from '@s4wave/web/router/router.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'

import { BillingStateProvider } from './BillingStateProvider.js'
import { BillingPage } from './BillingPage.js'

// BillingAccountDetailRoute reads the baId route param and renders the
// billing detail view scoped to that BillingAccount.
export function BillingAccountDetailRoute() {
  const params = useParams()
  const baId = params.baId ?? ''
  return (
    <BillingStateProvider billingAccountId={baId}>
      <SessionFrame>
        <BillingPage />
      </SessionFrame>
    </BillingStateProvider>
  )
}
