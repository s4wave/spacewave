import type { ReactNode } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { BillingStateProvider } from '@s4wave/app/billing/BillingStateProvider.js'
import { SubscriptionLapseBanner } from './SubscriptionLapseBanner.js'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// SpacewaveSessionContent wraps session children with the onboarding context,
// org list context, and subscription lapse banner for spacewave sessions.
export function SpacewaveSessionContent(props: {
  onboarding: WatchOnboardingStatusResponse | null
  children: ReactNode
}) {
  const sessionResource = SessionContext.useContext()

  const orgListResource = useStreamingResource(
    sessionResource,
    (session, signal) => session.spacewave.watchOrganizations(signal),
    [],
  )

  return (
    <SpacewaveOnboardingContext.Provider onboarding={props.onboarding}>
      <SpacewaveOrgListContext.Provider
        response={orgListResource.value ?? null}
        loading={orgListResource.loading}
      >
        <BillingStateProvider>
          <SubscriptionLapseBanner />
          {props.children}
        </BillingStateProvider>
      </SpacewaveOrgListContext.Provider>
    </SpacewaveOnboardingContext.Provider>
  )
}
