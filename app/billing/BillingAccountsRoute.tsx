import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'

import { BillingAccountsPage } from './BillingAccountsPage.js'

// BillingAccountsRoute renders the billing-account list within the session
// frame so the session bottom bar remains available on the billing list page.
export function BillingAccountsRoute() {
  return (
    <SessionFrame>
      <BillingAccountsPage />
    </SessionFrame>
  )
}
