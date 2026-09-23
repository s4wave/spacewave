import { FrontendResource } from '@aptre/bldr'
import type { Session } from '@go/github.com/s4wave/spacewave/bldr/frontend/frontend.pb.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { Space } from '@s4wave/sdk/space/space.js'
import type { BuildSpacePluginRequest } from '@s4wave/sdk/space/space.pb.js'

/** PluginFrontendSession is a compiler capability retained by its authoring view. */
export interface PluginFrontendSession {
  executionKey: string
  session: Session
  transport: FrontendResource
}

/** usePluginFrontend retains one device compiler and reports terminal stream errors. */
export function usePluginFrontend(
  space: Resource<Space>,
  request: BuildSpacePluginRequest,
): Resource<PluginFrontendSession> {
  return useStreamingResource(
    space,
    async function* (value, signal) {
      // React Refresh must attach before this document loads its renderer.
      if (!globalThis.__bldrFrontendEnabled) {
        throw new Error(
          'Live preview requires a Bldr development client. Open this Space with frontend development enabled.',
        )
      }

      // The hook owns both the remote execution grant and Vite's local transport.
      using attachment = await value.openPluginFrontend(request, signal)
      const ended = Promise.withResolvers<never>()
      void ended.promise.catch(() => {})
      using transport = new FrontendResource(
        attachment.frontend,
        ended.reject,
        false,
      )
      const abort = () => {
        transport.release()
        ended.reject(signal.reason)
      }
      signal.addEventListener('abort', abort, { once: true })
      if (signal.aborted) {
        abort()
      }

      // Keep consuming the lifetime after readiness so failures reach the view.
      try {
        const session = await transport.getSession()
        yield { executionKey: attachment.executionKey, session, transport }
        await ended.promise
      } finally {
        signal.removeEventListener('abort', abort)
      }
    },
    [request],
  )
}
