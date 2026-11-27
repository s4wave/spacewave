// Shared mounts of the Session, SharedObject, and Space resources.
//
// Every tab, the command scope, and the kiosk display open these through one
// entry per key, so a second tab on the same Session or Space reuses the
// mount instead of mounting again. Tabs on different Session indexes use
// different keys and keep separate mounts.

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useSharedResource,
  type UseSharedResourceOptions,
} from '@aptre/bldr-sdk/hooks/useSharedResource.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type {
  SharedObject,
  SharedObjectBody,
} from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { markQuickstartStartupBoundary } from '@s4wave/app/quickstart/startup-boundary.js'

/** sessionMountName selects the Session entry for sessionIdx under the Root. */
export function sessionMountName(sessionIdx: number): string {
  return `session/${sessionIdx}`
}

/** sharedObjectMountName selects the SharedObject entry under its Session. */
export function sharedObjectMountName(sharedObjectId: string): string {
  return `so/${sharedObjectId}`
}

/** bodyMountName selects the SharedObjectBody entry under its SharedObject. */
export const bodyMountName = 'body'

/** spaceMountName selects the Space entry under its SharedObjectBody. */
export const spaceMountName = 'space'

/** spaceWorldMountName selects the world state entry under its Space. */
export const spaceWorldMountName = 'world'

/** spaceContentsMountName selects the SpaceContents entry under its Space. */
export const spaceContentsMountName = 'contents'

// markMount records a startup boundary for a mount step and, when quickstart
// timing logs are on, prints it.
function markMount(label: string, fields: Record<string, unknown>): void {
  markQuickstartStartupBoundary(label, fields)
  if (
    (globalThis as { __s4waveLogQuickstartTiming?: boolean })
      .__s4waveLogQuickstartTiming
  ) {
    console.log(label + ': ' + JSON.stringify(fields))
  }
}

/**
 * useSessionMount mounts the Session at sessionIdx. A null value after
 * loading means the index does not exist. A non-empty generation remounts
 * the Session once after it moves to another provider account.
 */
export function useSessionMount(
  root: Resource<Root>,
  sessionIdx: number | null,
  options?: UseSharedResourceOptions<Session>,
): Resource<Session> {
  return useSharedResource(
    root,
    sessionIdx ? sessionMountName(sessionIdx) : null,
    async (root, signal, cleanup) => {
      // Mount the Session. A missing index resolves to null.
      if (!sessionIdx) return null
      markMount('quickstart.session-mount-start', { sessionIdx })
      const result = await root.mountSessionByIdx({ sessionIdx }, signal)
      if (!result) return null
      markMount('quickstart.session-mount-ready', {
        sessionIdx,
        released: result.session.released,
      })
      return cleanup(result.session)
    },
    options,
  )
}

/**
 * useSharedObjectMount mounts the SharedObject with sharedObjectId. A null
 * value after loading means the Session has no such SharedObject.
 */
export function useSharedObjectMount(
  session: Resource<Session>,
  sharedObjectId: string,
): Resource<SharedObject> {
  return useSharedResource(
    session,
    sharedObjectId ? sharedObjectMountName(sharedObjectId) : null,
    async (session, signal, cleanup) => {
      markMount('quickstart.shared-object-mount-start', { sharedObjectId })
      const result = await session.mountSharedObject({ sharedObjectId }, signal)
      markMount('quickstart.shared-object-mount-ready', {
        sharedObjectId,
        found: !!result,
      })
      return result ? cleanup(result) : null
    },
  )
}

/** useSharedObjectBodyMount mounts the body of a SharedObject. */
export function useSharedObjectBodyMount(
  sharedObject: Resource<SharedObject>,
): Resource<SharedObjectBody> {
  return useSharedResource(
    sharedObject,
    bodyMountName,
    async (sharedObject, signal, cleanup) => {
      // Mount the body and mark both ends of the mount.
      const sharedObjectId = sharedObject.meta.sharedObjectId
      markMount('quickstart.shared-object-body-mount-start', { sharedObjectId })
      const body = await sharedObject.mountSharedObjectBody({}, signal)
      markMount('quickstart.shared-object-body-mount-ready', { sharedObjectId })
      return cleanup(body)
    },
  )
}

/** useSpaceMount opens the Space a SharedObjectBody holds. */
export function useSpaceMount(
  body: Resource<SharedObjectBody>,
): Resource<Space> {
  return useSharedResource(body, spaceMountName, (body, _signal, cleanup) => {
    const space = cleanup(new Space(body.resourceRef.createRef(body.id)))
    markMount('quickstart.space-resource-created', { bodyResourceId: body.id })
    return Promise.resolve(space)
  })
}

/** useSpaceWorldMount opens the writable world state of a Space. */
export function useSpaceWorldMount(
  space: Resource<Space>,
): Resource<EngineWorldState> {
  return useSharedResource(
    space,
    spaceWorldMountName,
    async (space, signal, cleanup) => {
      markMount('quickstart.space-world-access-start', {
        spaceResourceId: space.id,
      })
      const world = await space.accessWorldState(true, signal)
      markMount('quickstart.space-world-access-ready', {
        spaceResourceId: space.id,
      })
      return cleanup(world)
    },
  )
}

/** useSpaceContentsMount mounts the contents of a Space. */
export function useSpaceContentsMount(
  space: Resource<Space>,
): Resource<SpaceContents> {
  return useSharedResource(
    space,
    spaceContentsMountName,
    async (space, signal, cleanup) => {
      markMount('quickstart.space-contents-mount-start', {
        spaceResourceId: space.id,
      })
      const contents = await space.mountSpaceContents(signal)
      markMount('quickstart.space-contents-mount-ready', {
        spaceResourceId: space.id,
      })
      return cleanup(contents)
    },
  )
}
