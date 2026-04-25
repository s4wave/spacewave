import { useMemo } from 'react'

import { hasInteracted } from '@s4wave/web/state/interaction.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { Landing } from './Landing.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'

// LandingOrRedirect checks for existing sessions and redirects accordingly.
export function LandingOrRedirect() {
  if (!hasInteracted()) {
    return <Landing />
  }
  return <LandingWithSessionCheck />
}

// LandingWithSessionCheck loads the session list and redirects if sessions exist.
function LandingWithSessionCheck() {
  const resource = useSessionList()
  const rootResource = useRootResource()

  // Load metadata for all sessions to determine provider types.
  const metaResource = useResource(
    rootResource,
    async (root, signal) => {
      const sessions = resource.value?.sessions ?? []
      if (sessions.length < 2) return null
      const results: Array<{
        session: SessionListEntry
        providerId: string
      }> = []
      for (const s of sessions) {
        if (!s.sessionIndex) continue
        const resp = await root.getSessionMetadata(s.sessionIndex, signal)
        results.push({
          session: s,
          providerId: resp.metadata?.providerId ?? '',
        })
      }
      return results
    },
    [resource.value?.sessions],
  )

  // Find the preferred local session when both local and cloud exist.
  const preferredLocalIdx = useMemo(() => {
    const entries = metaResource.value
    if (!entries || entries.length < 2) return null
    const local = entries.find((e) => e.providerId === 'local')
    const cloud = entries.find((e) => e.providerId === 'spacewave')
    if (local && cloud) {
      return local.session.sessionIndex
    }
    return null
  }, [metaResource.value])

  if (resource.loading) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Verifying sessions',
              detail: 'Checking for an existing Spacewave session.',
            }}
          />
        </div>
      </div>
    )
  }

  if (resource.error) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Failed to load sessions',
              error: resource.error.message,
              onRetry: resource.retry,
            }}
          />
        </div>
      </div>
    )
  }

  const sessions = resource.value?.sessions ?? []

  if (sessions.length === 0) {
    return <Landing />
  }

  if (sessions.length === 1) {
    return <NavigatePath to={'/u/' + sessions[0].sessionIndex + '/'} replace />
  }

  // When both local and cloud sessions exist, prefer the local session.
  // The cloud session's dormant/active status will prompt migration inside
  // SessionContainer via the spacewave onboarding watch.
  if (preferredLocalIdx) {
    return <NavigatePath to={'/u/' + preferredLocalIdx + '/'} replace />
  }

  // Still loading metadata for the preference check.
  if (metaResource.loading) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Choosing session',
              detail: 'Reading session metadata to pick the right redirect.',
            }}
          />
        </div>
      </div>
    )
  }

  return <NavigatePath to="/sessions" replace />
}
