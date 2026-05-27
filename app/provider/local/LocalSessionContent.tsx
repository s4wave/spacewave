import type { ReactNode } from 'react'

import { SetupBanner } from '@s4wave/app/session/SetupBanner.js'
import { SessionStorageStatsProvider } from '@s4wave/app/session/SessionStorageStatsContext.js'
import { LocalSessionOnboardingProvider } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

// LocalSessionContent wraps session children with local onboarding state and the setup banner.
export function LocalSessionContent(props: {
  metadata?: SessionMetadata
  children: ReactNode
}) {
  return (
    <SessionStorageStatsProvider>
      <LocalSessionOnboardingProvider metadata={props.metadata}>
        <SetupBanner />
        {props.children}
      </LocalSessionOnboardingProvider>
    </SessionStorageStatsProvider>
  )
}
