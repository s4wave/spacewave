import { mountSpace } from '@s4wave/app/space/space.js'
import { getDebugContext } from '@s4wave/sdk/debug/context.js'
import type { Root } from '@s4wave/sdk/root'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'

import { type CleanupTracker, withCleanup } from './test-utils.js'

// SpaceTarget names a Space by its session index and shared object ID.
export interface SpaceTarget {
  sessionIdx: number
  sharedObjectId: string
}

// currentSpaceTarget reads the Space the page's /u/{idx}/so/{id} route shows.
export function currentSpaceTarget(): SpaceTarget {
  const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
  if (!match) {
    throw new Error('expected current /u/{idx}/so/{id} route')
  }
  return {
    sessionIdx: Number(match[1]),
    sharedObjectId: decodeURIComponent(match[2] ?? ''),
  }
}

// withMountedWorld mounts the target Space's World through the app's debug
// root, calls fn with it, and releases everything mounted, including what fn
// tracked with cleanup, when fn settles.
export async function withMountedWorld<T>(
  target: SpaceTarget,
  write: boolean,
  signal: AbortSignal,
  fn: (world: EngineWorldState, cleanup: CleanupTracker) => Promise<T>,
): Promise<T> {
  const { root } = getDebugContext<{ root?: Root }>()
  if (!root) {
    throw new Error('debug context has no root')
  }
  return await withCleanup(async (cleanup) => {
    const mounted = await root.mountSessionByIdx(
      { sessionIdx: target.sessionIdx },
      signal,
    )
    const session = cleanup(mounted?.session)
    if (!session) {
      throw new Error('mountSessionByIdx returned no session')
    }
    const space = cleanup(
      await mountSpace({
        session,
        spaceResp: {
          sharedObjectRef: {
            providerResourceRef: { id: target.sharedObjectId },
          },
        },
        abortSignal: signal,
        cleanup,
      }),
    )
    const world = cleanup(await space.accessWorldState(write, signal))
    return await fn(world, cleanup)
  })
}
