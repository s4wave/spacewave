import type { ReactNode } from 'react'

import { SpacewaveSessionContent } from '@s4wave/app/provider/spacewave/SpacewaveSessionContent.js'
import { LocalSessionContent } from '@s4wave/app/provider/local/LocalSessionContent.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// SessionProviderContainer dispatches to provider-specific content wrappers
// based on the session's provider ID. Spacewave sessions get onboarding
// context and lapse banner; local sessions get the setup banner.
export function SessionProviderContainer(props: {
  metadata?: SessionMetadata
  spacewaveOnboarding?: WatchOnboardingStatusResponse | null
  children: ReactNode
}) {
  switch (props.metadata?.providerId) {
    case 'spacewave':
      return (
        <SpacewaveSessionContent onboarding={props.spacewaveOnboarding ?? null}>
          {props.children}
        </SpacewaveSessionContent>
      )
    case 'local':
      return (
        <LocalSessionContent metadata={props.metadata}>
          {props.children}
        </LocalSessionContent>
      )
    default:
      return <>{props.children}</>
  }
}
