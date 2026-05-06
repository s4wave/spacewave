import type { WatchSpaceRootRuntimeResponse } from '@s4wave/sdk/root/root.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'

const EMPTY_ROOT_RUNTIME: Resource<WatchSpaceRootRuntimeResponse> = {
  loading: false,
  value: null,
  error: null,
  retry: () => {},
}

async function* emptyRuntimeStream(): AsyncIterable<WatchSpaceRootRuntimeResponse> {}

// useSpaceRootRuntime returns sessions from the selected configured state root.
export function useSpaceRootRuntime(
  aliasId: string | null,
): Resource<WatchSpaceRootRuntimeResponse> {
  const isStatic = useIsStaticMode()
  const rootResource = useRootResource()
  const resource = useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) => {
      if (!aliasId) return emptyRuntimeStream()
      return root.watchSpaceRootRuntime({ aliasId, autostart: true }, signal)
    },
    [aliasId],
  )
  if (isStatic) return EMPTY_ROOT_RUNTIME
  if (!aliasId) return EMPTY_ROOT_RUNTIME
  return resource
}
