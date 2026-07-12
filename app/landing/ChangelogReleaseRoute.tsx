import { useMemo } from 'react'

import { useParams } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { ChangelogReleasePage } from './ChangelogReleasePage.js'

// ChangelogReleaseRoute resolves the version route param against the loaded
// changelog and renders that release's page, redirecting to /changelog when
// the version is unknown.
export function ChangelogReleaseRoute() {
  const params = useParams()
  const version = (params['version'] ?? '').replace(/^v/, '')

  const rootResource = useRootResource()
  const changelogResource = useResource(
    rootResource,
    async (root, signal) => root.getChangelog(signal),
    [],
  )
  const releases = useMemo(
    () => changelogResource.value?.releases ?? [],
    [changelogResource.value],
  )
  const release = useMemo(
    () => releases.find((r) => r.version === version),
    [releases, version],
  )

  if (changelogResource.loading) {
    return (
      <div className="bg-background-landing flex w-full flex-1 items-center justify-center">
        <div className="w-full max-w-sm px-4">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading release',
              detail: `Fetching Spacewave v${version}.`,
            }}
          />
        </div>
      </div>
    )
  }

  if (!release) {
    return <NavigatePath to="/changelog" replace />
  }

  return <ChangelogReleasePage release={release} />
}
