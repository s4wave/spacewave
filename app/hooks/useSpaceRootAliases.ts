import type { WatchSpaceRootAliasesResponse } from '@s4wave/sdk/root/root.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'

const EMPTY_ROOT_ALIASES: Resource<WatchSpaceRootAliasesResponse> = {
  loading: false,
  value: { records: [] },
  error: null,
  retry: () => {},
}

// useSpaceRootAliases returns configured local state-root records.
export function useSpaceRootAliases(): Resource<WatchSpaceRootAliasesResponse> {
  const isStatic = useIsStaticMode()
  const rootResource = useRootResource()
  const resource = useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) =>
      root.watchSpaceRootAliases({}, signal),
    [],
  )
  if (isStatic) return EMPTY_ROOT_ALIASES
  return resource
}
