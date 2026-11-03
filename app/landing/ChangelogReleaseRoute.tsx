import { useMemo } from 'react'

import { useParams } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { ChangelogReleasePage } from './ChangelogReleasePage.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    justifyContent: 'center',
    width: '100%',
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
})

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
      <div sx={stylexStyles.rule1}>
        <div sx={stylexStyles.rule2}>
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
