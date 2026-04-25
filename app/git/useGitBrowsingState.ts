import { useMemo } from 'react'

import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePath } from '@s4wave/web/router/router.js'
import { useStateNamespace } from '@s4wave/web/state/persist.js'

import { parseViewerRoute } from './layout/route.js'

// useGitBrowsingState encapsulates the shared browsing state for git viewers:
// route parsing, effectiveRef resolution, tip commit lookup, and immutable tree
// FSHandle.
export function useGitBrowsingState(
  repoHandle: Resource<GitRepoHandle>,
  headRef: string | null | undefined,
  stateNs: string,
) {
  const path = usePath()

  useStateNamespace([stateNs])

  // Parse viewer path into route components (GitHub-style URLs).
  const route = useMemo(() => parseViewerRoute(path), [path])

  // Resolve effective ref: from URL or fallback headRef.
  const effectiveRef = useMemo(() => {
    if (route.ref) return route.ref
    return headRef ?? null
  }, [route.ref, headRef])

  // Fetch tip commit for the selected ref or specific commit hash.
  const tipCommitResource = useResource(
    repoHandle,
    async (repo) => {
      if (!repo || !effectiveRef) return null
      if (route.commitHash) {
        return (await repo.getCommit(route.commitHash)) ?? null
      }
      const resp = await repo.log(effectiveRef, 0, 1)
      return resp.commits?.[0] ?? null
    },
    [effectiveRef, route.commitHash],
  )

  // Immutable tree FSHandle for the selected ref.
  const rootHandleResource = useResource(
    repoHandle,
    async (repo, signal, cleanup) => {
      if (!repo || !effectiveRef) return null
      return cleanup(await repo.getTreeResource(effectiveRef, signal))
    },
    [effectiveRef],
  )

  return {
    route,
    effectiveRef,
    tipCommitResource,
    rootHandleResource,
  }
}
